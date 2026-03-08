package control

const (
	DefaultBaseScoreWeight    = 0.70
	DefaultNetworkScoreWeight = 0.30
)

func CombineScores(baseScore, networkScore float64) float64 {
	if baseScore < 0 {
		baseScore = 0
	}
	if networkScore < 0 {
		networkScore = 0
	}

	return (DefaultBaseScoreWeight * baseScore) + (DefaultNetworkScoreWeight * networkScore)
}
