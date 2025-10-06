package capture

import (
	"context"
	"sync"
)

// StateChange represents an observable controller state transition.
type StateChange struct {
	State  string
	Reason string
}

// Controller coordinates pause/resume/kill signals across subsystems.
type Controller struct {
	mu        sync.Mutex
	paused    bool
	stopping  bool
	stopErr   error
	signal    chan struct{}
	watchers  map[int]chan StateChange
	nextWatch int
}

// NewController constructs a controller in the running state.
func NewController() *Controller {
	return &Controller{signal: make(chan struct{}), watchers: make(map[int]chan StateChange)}
}

// Pause transitions the controller into a paused state.
func (c *Controller) Pause() {
	c.mu.Lock()
	if c.paused {
		c.mu.Unlock()
		return
	}
	c.paused = true
	c.broadcastLocked(StateChange{State: "paused", Reason: "pause requested"})
	c.mu.Unlock()
}

// Resume clears a paused state and notifies waiters.
func (c *Controller) Resume() {
	c.mu.Lock()
	alreadyRunning := !c.paused
	c.paused = false
	if !alreadyRunning {
		c.broadcastLocked(StateChange{State: "running", Reason: "resumed"})
		c.notifyAllLocked()
	}
	c.mu.Unlock()
}

// Kill requests subsystems to stop and propagates an optional error.
func (c *Controller) Kill(err error) {
	c.mu.Lock()
	alreadyStopping := c.stopping
	if !c.stopping {
		c.stopping = true
	}
	if err != nil && c.stopErr == nil {
		c.stopErr = err
	}
	if !alreadyStopping {
		reason := ""
		if err != nil {
			reason = err.Error()
		}
		c.broadcastLocked(StateChange{State: "stopping", Reason: reason})
		c.notifyAllLocked()
	}
	c.mu.Unlock()
}

// Wait blocks until the controller is running or stopping.
func (c *Controller) Wait(ctx context.Context) error {
	for {
		c.mu.Lock()
		paused := c.paused
		stopping := c.stopping
		stopErr := c.stopErr
		signal := c.signal
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
			<-signal
			continue
		}

		select {
		case <-ctx.Done():
			c.Kill(ctx.Err())
			return ctx.Err()
		case <-signal:
			continue
		}
	}
}

// State reports the textual state for diagnostics.
func (c *Controller) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentStateLocked()
}

// Subscribe registers a watcher for state transitions.
func (c *Controller) Subscribe() (<-chan StateChange, func()) {
	ch := make(chan StateChange, 4)
	c.mu.Lock()
	id := c.nextWatch
	c.nextWatch++
	c.watchers[id] = ch
	initial := c.currentStateLocked()
	c.mu.Unlock()

	ch <- StateChange{State: initial, Reason: "initial"}

	cancel := func() {
		c.mu.Lock()
		if watcher, ok := c.watchers[id]; ok {
			delete(c.watchers, id)
			close(watcher)
		}
		c.mu.Unlock()
	}
	return ch, cancel
}

func (c *Controller) broadcastLocked(change StateChange) {
	for _, watcher := range c.watchers {
		select {
		case watcher <- change:
		default:
		}
	}
}

func (c *Controller) currentStateLocked() string {
	switch {
	case c.stopping:
		return "stopping"
	case c.paused:
		return "paused"
	default:
		return "running"
	}
}

func (c *Controller) notifyAllLocked() {
	close(c.signal)
	c.signal = make(chan struct{})
}
