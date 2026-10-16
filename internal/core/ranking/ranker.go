package ranking

import (
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"math"
	"sort"
	"time"
)

type ServiceEvidence struct {
	Success   bool
	Latency   time.Duration
	Failures  int
	Timeouts  int
	Stability float64
}

type StaticRanker struct{}
type DynamicRanker struct{}
type RecoveryRanker struct{}

func (StaticRanker) Score(r model.FinalResult) float64 {
	latency := float64(r.Latency.Milliseconds())
	if latency < 1 {
		latency = 1
	}
	latencyScore := 1000.0 / (1.0 + latency)
	return latencyScore + r.SuccessRate*500 + float64(r.StabilityScore)*2
}
func (DynamicRanker) Score(base float64, failures, timeouts int, switches int, lastSuccess time.Time) float64 {
	agePenalty := 0.0
	if !lastSuccess.IsZero() {
		agePenalty = math.Min(200, time.Since(lastSuccess).Minutes())
	}
	return base - float64(failures*80+timeouts*100+switches*25) - agePenalty
}
func (RecoveryRanker) Score(static, dynamic float64, serviceSuccess float64, historyFailures int) float64 {
	return static*0.35 + dynamic*0.45 + serviceSuccess*300 - float64(historyFailures*20)
}

type NodeRanker struct {
	Static   StaticRanker
	Dynamic  DynamicRanker
	Recovery RecoveryRanker
}

func (r NodeRanker) Rank(results []model.FinalResult) {
	for i := range results {
		results[i].Score = int(r.Static.Score(results[i]))
		results[i].Score = int(r.Recovery.Score(float64(results[i].Score), float64(results[i].Score), results[i].SuccessRate, 0))
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Latency != results[j].Latency {
			return results[i].Latency < results[j].Latency
		}
		return results[i].Node.Host+":"+results[i].Node.Port < results[j].Node.Host+":"+results[j].Node.Port
	})
}
