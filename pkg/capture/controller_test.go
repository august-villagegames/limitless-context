package capture

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestControllerPauseResume(t *testing.T) {
	controller := NewController()

	controller.Pause()
	done := make(chan error, 1)
	go func() {
		done <- controller.Wait(context.Background())
	}()

	select {
	case <-time.After(100 * time.Millisecond):
	case err := <-done:
		t.Fatalf("expected wait to block, got %v", err)
	}

	controller.Resume()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error after resume, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("controller wait did not resume")
	}
}

func TestControllerKillPropagatesError(t *testing.T) {
	controller := NewController()
	customErr := errors.New("boom")

	done := make(chan error, 1)
	go func() {
		done <- controller.Wait(context.Background())
	}()

	controller.Kill(customErr)

	select {
	case err := <-done:
		if !errors.Is(err, customErr) {
			t.Fatalf("expected custom error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("controller wait did not unblock after kill")
	}
}

func TestControllerWaitRespectsContextCancellation(t *testing.T) {
	controller := NewController()
	controller.Pause()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- controller.Wait(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("controller wait did not exit on cancellation")
	}
}
