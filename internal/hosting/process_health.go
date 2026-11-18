package hosting

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Probe only the owned loopback endpoint. An application health failure never
// changes permissions, installs a runtime, or contacts a supplied remote URL.
func (a *App) watchNodeHealth(ctx context.Context, port int, path string) <-chan struct{} {
	failed := make(chan struct{})
	go func() {
		interval, grace := 10*time.Second, 45*time.Second
		if a.healthTestInterval > 0 {
			interval, grace = a.healthTestInterval, a.healthTestGrace
		}
		started, misses := time.Now(), 0
		client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
			ok := false
			if err == nil {
				response, requestErr := client.Do(req)
				if requestErr == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
					response.Body.Close()
					ok = response.StatusCode >= 200 && response.StatusCode < 400
				}
			}
			if ok {
				misses = 0
			} else if time.Since(started) >= grace {
				misses++
			}
			if misses >= 3 {
				close(failed)
				return
			}
		}
	}()
	return failed
}
