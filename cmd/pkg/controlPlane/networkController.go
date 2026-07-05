package control

import (
	"math"
	"sync"
	"time"
	"fmt"
	"net/http"
	"bytes"
	"encoding/json"
	"github.com/go-ping/ping"
)

const (
	DefaultBaseScoreWeight    = 0.70
	DefaultNetworkScoreWeight = 0.30
) 

type IperfResult struct {
    End struct {
        SumReceived struct {
            BitsPerSecond float64 `json:"bits_per_second"`
        } `json:"sum_received"`
    } `json:"end"`
}


type LinkMetrics struct {
	Latency       time.Duration `json:"latency"`
	BandwidthMbps float64       `json:"bandwidth_mbps"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type NetworkController struct {
	mu                   sync.RWMutex
	links                map[string]map[string]LinkMetrics
	defaultLatency       time.Duration
	defaultBandwidthMbps float64
}

func CombineScores(baseScore, networkScore float64) float64 {
	if baseScore < 0 {
		baseScore = 0
	}
	if networkScore < 0 {
		networkScore = 0
	}

	return (DefaultBaseScoreWeight * baseScore) + (DefaultNetworkScoreWeight * networkScore)
}

func NewNetworkController() *NetworkController {
	return &NetworkController{
		links:                make(map[string]map[string]LinkMetrics),
		defaultLatency:       25 * time.Millisecond,
		defaultBandwidthMbps: 250,
	}
}

func (n *NetworkController)MeasureLatency(host string) time.Duration {
	pinger, err := ping.NewPinger(host)
	if err != nil {
		return 25 * time.Millisecond
	}
	pinger.Count = 3
	pinger.Timeout = 3 * time.Second
	pinger.SetPrivileged(true)
	err = pinger.Run()
	if err != nil {
		return 25 * time.Millisecond
	}
	stats := pinger.Statistics()
	return stats.AvgRtt
}

func (n *NetworkController) MeasureBandwidth(srcAddr, dstAddr string) float64 {
	url := fmt.Sprintf("http://%s/measure-bandwidth", srcAddr)
	reqBody := map[string]string{
		"target": dstAddr,
	}
	data, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var result struct {
		Bandwidth float64 `json:"bandwidth"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return 0
	}
	return result.Bandwidth
}

func (n *NetworkController) SetDefaults(latency time.Duration, bandwidthMbps float64) {
	if n == nil {
		return
	}
	if latency <= 0 {
		latency = 25 * time.Millisecond
	}
	if bandwidthMbps <= 0 {
		bandwidthMbps = 250
	}

	n.mu.Lock()
	n.defaultLatency = latency
	n.defaultBandwidthMbps = bandwidthMbps
	n.mu.Unlock()
}

func (n *NetworkController) UpdateLink(srcID, dstID string, latency time.Duration, bandwidthMbps float64) {
	if n == nil || srcID == "" || dstID == "" || srcID == dstID {
		return
	}
	if latency <= 0 {
		latency = n.defaultLatency
	}
	if bandwidthMbps <= 0 {
		bandwidthMbps = n.defaultBandwidthMbps
	}

	metric := LinkMetrics{Latency: latency, BandwidthMbps: bandwidthMbps, UpdatedAt: time.Now().UTC()}

	n.mu.Lock()
	if _, ok := n.links[srcID]; !ok {
		n.links[srcID] = make(map[string]LinkMetrics)
	}
	if _, ok := n.links[dstID]; !ok {
		n.links[dstID] = make(map[string]LinkMetrics)
	}
	n.links[srcID][dstID] = metric
	n.links[dstID][srcID] = metric
	n.mu.Unlock()
}

func (n *NetworkController) GetLink(srcID, dstID string) (LinkMetrics, bool) {
	if n == nil || srcID == "" || dstID == "" {
		return LinkMetrics{}, false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	if neighbors, ok := n.links[srcID]; ok {
		metric, exists := neighbors[dstID]
		if exists {
			return metric, true
		}
	}

	return LinkMetrics{}, false
}

func (n *NetworkController) EnsureDefaultLinks(nodeIDs []string) {
	if n == nil || len(nodeIDs) < 2 {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now().UTC()
	for i := 0; i < len(nodeIDs); i++ {
		src := nodeIDs[i]
		if src == "" {
			continue
		}
		if _, ok := n.links[src]; !ok {
			n.links[src] = make(map[string]LinkMetrics)
		}

		for j := i + 1; j < len(nodeIDs); j++ {
			dst := nodeIDs[j]
			if dst == "" || dst == src {
				continue
			}
			if _, ok := n.links[dst]; !ok {
				n.links[dst] = make(map[string]LinkMetrics)
			}

			if _, exists := n.links[src][dst]; !exists {
				metric := LinkMetrics{
					Latency:       n.defaultLatency,
					BandwidthMbps: n.defaultBandwidthMbps,
					UpdatedAt:     now,
				}
				n.links[src][dst] = metric
				n.links[dst][src] = metric
			}
		}
	}
}

func (n *NetworkController) ScoreNode(nodeID string, peerIDs []string) float64 {
	if n == nil {
		return 1.0
	}
	if len(peerIDs) == 0 {
		return 1.0
	}

	n.mu.RLock()
	defaultLatency := n.defaultLatency
	defaultBandwidth := n.defaultBandwidthMbps
	n.mu.RUnlock()

	if defaultLatency <= 0 {
		defaultLatency = 25 * time.Millisecond
	}
	if defaultBandwidth <= 0 {
		defaultBandwidth = 250
	}

	total := 0.0
	count := 0.0

	for _, peerID := range peerIDs {
		if peerID == "" || peerID == nodeID {
			continue
		}

		metric, ok := n.GetLink(nodeID, peerID)
		if !ok {
			metric = LinkMetrics{Latency: defaultLatency, BandwidthMbps: defaultBandwidth}
		}

		latencyMs := float64(metric.Latency) / float64(time.Millisecond)
		if latencyMs < 0 {
			latencyMs = 0
		}

		latencyScore := 1.0 / (1.0 + (latencyMs / 10.0))
		bandwidthScore := metric.BandwidthMbps / defaultBandwidth
		if bandwidthScore < 0 {
			bandwidthScore = 0
		}
		bandwidthScore = math.Min(bandwidthScore, 1.0)

		total += 0.6*latencyScore + 0.4*bandwidthScore
		count++
	}

	if count == 0 {
		return 1.0
	}

	avg := total / count
	if avg < 0 {
		return 0
	}
	if avg > 1 {
		return 1
	}

	return avg
}