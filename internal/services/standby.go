package services

import (
	"context"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/checker"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"time"
)

func probeStandby(ctx context.Context, binary, root, service string, specURL string, entries []*PoolEntry) ([]model.FinalResult, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	svc := map[string]string{service: specURL}
	d := checker.NewDeepChecker(binary, root, 1, 12*time.Second, svc)
	in := make(chan model.NodeInfo, len(entries))
	for _, e := range entries {
		if e != nil {
			in <- e.Node
		}
	}
	close(in)
	out := d.StreamDeepCheck(ctx, in, nil)
	results := make([]model.FinalResult, 0, len(entries))
	for r := range out {
		results = append(results, r)
	}
	return results, nil
}
