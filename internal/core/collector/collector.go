package collector

import (
	"context"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
	"strings"
)

// Stream forwards fetched lines without building a global node slice.
func Stream(ctx context.Context, in <-chan string, pm *progress.Manager) <-chan string {
	out := make(chan string, 2048)
	if pm != nil {
		pm.AddStage("COLLECTOR", progress.UnknownTotal, 1)
		pm.StartStage("COLLECTOR")
	}
	go func() {
		defer close(out)
		defer func() {
			if pm != nil {
				pm.Complete("COLLECTOR")
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-in:
				if !ok {
					return
				}
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
					if pm != nil {
						pm.Fail("COLLECTOR")
					}
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- line:
					if pm != nil {
						pm.Success("COLLECTOR")
						pm.Produce("COLLECTOR", 1)
					}
				}
			}
		}
	}()
	return out
}
