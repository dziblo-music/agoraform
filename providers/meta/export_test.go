package meta

import (
	"context"
	"time"
)

// SetVideoPollingForTest overrides video processing wait bounds in tests.
func SetVideoPollingForTest(p *Provider, timeout, interval time.Duration, sleep func(context.Context, time.Duration) error) {
	if p == nil {
		return
	}
	p.videoReadyTimeout = timeout
	p.videoPollInterval = interval
	p.sleep = sleep
}
