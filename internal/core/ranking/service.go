package ranking

import (
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"sort"
	"time"
)

type ServiceScore struct {
	Service string
	Node    model.NodeInfo
	Score   float64
	Latency time.Duration
	Success bool
}

func RankService(nodes []model.FinalResult, service string) []ServiceScore {
	out := make([]ServiceScore, 0, len(nodes))
	for _, r := range nodes {
		ok := r.SuccessServices[service]
		lat := r.ServiceLatency[service]
		score := 0.0
		if ok {
			score = 1000.0 / (1 + float64(lat.Milliseconds()))
			score += r.SuccessRate * 250
		}
		out = append(out, ServiceScore{Service: service, Node: r.Node, Score: score, Latency: lat, Success: ok})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
