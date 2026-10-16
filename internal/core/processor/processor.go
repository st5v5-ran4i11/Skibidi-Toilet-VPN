package processor

import (
	"context"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
	"strings"
)

// Stream normalizes subscription lines and keeps processing bounded by the channel.
func Stream(ctx context.Context, in <-chan string, pm *progress.Manager) <-chan string {
	out := make(chan string, 2048)
	if pm != nil {
		pm.AddStage("PROCESSOR", progress.UnknownTotal, 1)
		pm.StartStage("PROCESSOR")
	}
	go func() {
		defer close(out)
		defer func() {
			if pm != nil {
				pm.Complete("PROCESSOR")
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
				if line == "" {
					if pm != nil {
						pm.Fail("PROCESSOR")
					}
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- line:
					if pm != nil {
						pm.Success("PROCESSOR")
						pm.Produce("PROCESSOR", 1)
					}
				}
			}
		}
	}()
	return out
}
