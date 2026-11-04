package supervisor

import (
	"context"
	"fmt"
	"sync"
)

// Service runs until canceled or failed. It calls ready only after its own
// readiness checks and must stop all resources it owns before returning.
type Service interface {
	Name() string
	Run(context.Context, func()) error
}
type ServiceFunc struct {
	Label   string
	Execute func(context.Context, func()) error
}

func (s ServiceFunc) Name() string                                { return s.Label }
func (s ServiceFunc) Run(ctx context.Context, ready func()) error { return s.Execute(ctx, ready) }

// RunServices waits for every service before declaring readiness. A service
// failure cancels its siblings; their cleanup completes before this returns.
func RunServices(parent context.Context, services []Service, onReady func()) error {
	if len(services) == 0 {
		return fmt.Errorf("no services configured")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	ready, done := make(chan struct{}, len(services)), make(chan result, len(services))
	for _, service := range services {
		service := service
		go func() {
			var once sync.Once
			err := service.Run(ctx, func() { once.Do(func() { ready <- struct{}{} }) })
			done <- result{service.Name(), err}
		}()
	}
	remaining, prepared := len(services), 0
	var failure error
	for remaining > 0 {
		select {
		case <-ready:
			prepared++
			if prepared == len(services) && ctx.Err() == nil {
				onReady()
			}
		case result := <-done:
			remaining--
			if ctx.Err() == nil {
				if result.err != nil {
					failure = fmt.Errorf("%s: %w", result.name, result.err)
				} else {
					failure = fmt.Errorf("%s exited unexpectedly", result.name)
				}
				cancel()
			}
		case <-ctx.Done():
			// Drain completions without repeatedly selecting a closed context channel.
			for remaining > 0 {
				result := <-done
				remaining--
				_ = result
			}
		}
	}
	if parent.Err() != nil {
		return nil
	}
	return failure
}
