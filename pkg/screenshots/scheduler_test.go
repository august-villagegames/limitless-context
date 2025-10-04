package screenshots

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSchedulerValidation(t *testing.T) {
	if _, err := NewScheduler(Options{Interval: 0, MaxPerMinute: 1}); err == nil {
		t.Fatalf("expected error for zero interval")
	}
	if _, err := NewScheduler(Options{Interval: time.Second, MaxPerMinute: 0}); err == nil {
		t.Fatalf("expected error for zero max per minute")
	}
}

func TestSchedulerCaptureRespectsThrottle(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler, err := NewScheduler(Options{Interval: 5 * time.Second, MaxPerMinute: 4, Clock: func() time.Time { return base }})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	dir := t.TempDir()
	result, err := scheduler.Capture(context.Background(), dir)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if result.Count != 4 {
		t.Fatalf("expected 4 captures, got %d", result.Count)
	}
	for i, path := range result.Files {
		expected := filepath.Join(dir, fmt.Sprintf("screenshot_%03d.txt", i+1))
		if path != expected {
			t.Fatalf("unexpected path %q, want %q", path, expected)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected screenshot file to exist: %v", err)
		}
	}
}

func TestSchedulerCaptureCancellation(t *testing.T) {
	scheduler, err := NewScheduler(Options{Interval: time.Second, MaxPerMinute: 2})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := scheduler.Capture(ctx, t.TempDir()); err == nil {
		t.Fatalf("expected cancellation error")
	}
}
