package capture

import (
	"context"
	"sync"
)

// Controller coordinates pause/resume/kill signals across subsystems.
type Controller struct {
	mu       sync.Mutex
	paused   bool
	stopping bool
	stopErr  error
	signal   chan struct{}
}

// NewController constructs a controller in the running state.
func NewController() *Controller {
	return &Controller{signal: make(chan struct{}, 1)}
}

// Pause transitions the controller into a paused state.
func (c *Controller) Pause() {
	c.mu.Lock()
	c.paused = true
	c.mu.Unlock()
}

// Resume clears a paused state and notifies waiters.
func (c *Controller) Resume() {
	c.mu.Lock()
	alreadyRunning := !c.paused
	c.paused = false
	c.mu.Unlock()
	if !alreadyRunning {
		c.notify()
	}
}

// Kill requests subsystems to stop and propagates an optional error.
func (c *Controller) Kill(err error) {
	c.mu.Lock()
	if !c.stopping {
		c.stopping = true
	}
	if err != nil && c.stopErr == nil {
		c.stopErr = err
	}
	c.mu.Unlock()
	c.notify()
}

// Wait blocks until the controller is running or stopping.
func (c *Controller) Wait(ctx context.Context) error {
	for {
		c.mu.Lock()
		paused := c.paused
		stopping := c.stopping
		stopErr := c.stopErr
		c.mu.Unlock()

		if stopping {
			if stopErr != nil {
				return stopErr
			}
			if ctx != nil && ctx.Err() != nil {
				return ctx.Err()
			}
			return context.Canceled
		}
		if !paused {
			return nil
		}

		if ctx == nil {
			<-c.signal
			continue
		}

		select {
		case <-ctx.Done():
			c.Kill(ctx.Err())
			return ctx.Err()
		case <-c.signal:
			continue
		}
	}
}

// State reports the textual state for diagnostics.
func (c *Controller) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.stopping:
		return "stopping"
	case c.paused:
		return "paused"
	default:
		return "running"
	}
}

func (c *Controller) notify() {
	select {
	case c.signal <- struct{}{}:
	default:
	}
}
