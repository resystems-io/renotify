package registry

import (
	"context"
	"time"
)

// runReaper periodically checks for stale flows and terminates
// them. The interval is configurable via reaping.interval
// (default 30s).
func (s *Service) runReaper(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.Interval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reapOnce()
		}
	}
}
