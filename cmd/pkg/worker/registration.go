package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"orchestrator/store"
	"time"
)

func RegisterWithManager(ctx context.Context, managerAddress string, meta store.Worker) error {
	meta.Heartbeat = time.Now().UTC()

	// Collect initial resource stats
	stats := GetStats()
	meta.CPUUsage = stats.CpuUsage() * 100 // Convert to percentage
	meta.MemoryAvailable = uint64(stats.MemAvailableKb())
	meta.MemoryTotal = uint64(stats.MemTotalKb())
	meta.DiskFree = uint64(stats.DiskFree())
	meta.DiskTotal = uint64(stats.DiskTotal())

	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s/workers", managerAddress), bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d during registration", resp.StatusCode)
	}

	return nil
}

func sendHeartbeat(ctx context.Context, managerAddress, workerID string) error {
	msg := fmt.Sprintf("http://%s/workers/%s/heartbeat", managerAddress, workerID)
	fmt.Printf("Sending heartbeat at url %s \n", msg)

	// Collect current resource stats
	stats := GetStats()

	// Prepare heartbeat payload with stats
	heartbeatData := map[string]interface{}{
		"cpu_usage":        stats.CpuUsage() * 100, // Convert to percentage
		"memory_available": stats.MemAvailableKb(),
		"memory_total":     stats.MemTotalKb(),
		"disk_free":        stats.DiskFree(),
		"disk_total":       stats.DiskTotal(),
	}

	payload, err := json.Marshal(heartbeatData)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, msg, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d during heartbeat", resp.StatusCode)
	}

	return nil
}

func StartHeartbeat(ctx context.Context, managerAddress, workerID string, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeatCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := sendHeartbeat(heartbeatCtx, managerAddress, workerID); err != nil {
				log.Printf("Error sending heartbeat for worker %s: %v", workerID, err)
			}
			cancel()
		}
	}
}
