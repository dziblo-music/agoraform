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

// SetDestroyConfirmForTest overrides the re-read schedule after DELETE.
func SetDestroyConfirmForTest(p *Provider, delays []time.Duration, sleep func(context.Context, time.Duration) error) {
	if p == nil {
		return
	}
	p.destroyConfirmDelays = append([]time.Duration{}, delays...)
	p.sleep = sleep
}
