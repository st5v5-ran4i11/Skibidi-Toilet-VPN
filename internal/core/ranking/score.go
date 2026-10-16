package ranking

import (
	"math"
	"time"
)

func CalculateScore(latency time.Duration) int {
	ms := float64(latency.Milliseconds())
	if ms <= 0 {
		return 200
	}
	score := 200.0 * math.Exp(-0.002*ms)
	if score < 10 {
		return 10
	}
	return int(score)
}
