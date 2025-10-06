package screenshots

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeProvider struct {
	frames []FrameCapture
	idx    int
}

func (f *fakeProvider) Grab(ctx context.Context) (FrameCapture, error) {
	if f.idx >= len(f.frames) {
		return FrameCapture{}, errors.New("no more frames")
	}
	frame := f.frames[f.idx]
	f.idx++
	return frame, nil
}

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

func (f *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	f.Advance(d)
	return nil
}

func TestNewSchedulerValidation(t *testing.T) {
	if _, err := NewScheduler(Options{Interval: 0, MaxPerMinute: 1, Provider: &fakeProvider{}}); err == nil {
		t.Fatalf("expected error for zero interval")
	}
	if _, err := NewScheduler(Options{Interval: time.Second, MaxPerMinute: 0, Provider: &fakeProvider{}}); err == nil {
		t.Fatalf("expected error for zero max per minute")
	}
}

func TestSchedulerCaptureProducesArtifacts(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: base}
	provider := &fakeProvider{
		frames: []FrameCapture{
			{
				PNG:      []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A},
				Metadata: Metadata{CapturedAt: base, Backend: "fake", Width: 10, Height: 10, PixelFormat: "FAKE"},
			},
			{
				PNG:      []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00},
				Metadata: Metadata{CapturedAt: base.Add(5 * time.Second), Backend: "fake", Width: 11, Height: 11, PixelFormat: "FAKE"},
			},
		},
	}

	scheduler, err := NewScheduler(Options{
		Interval:     5 * time.Second,
		MaxPerMinute: 2,
		Clock:        clock.Now,
		Provider:     provider,
		Sleeper:      clock.Sleep,
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	dir := t.TempDir()
	result, err := scheduler.Capture(context.Background(), dir)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if result.Count != 2 {
		t.Fatalf("expected 2 captures, got %d", result.Count)
	}
	if result.FirstCapture != base {
		t.Fatalf("unexpected first capture timestamp: %s", result.FirstCapture)
	}
	if result.LastCapture != base.Add(5*time.Second) {
		t.Fatalf("unexpected last capture timestamp: %s", result.LastCapture)
	}
	if len(result.Files) != 2 || len(result.MetadataFiles) != 2 {
		t.Fatalf("expected two files and metadata entries")
	}

	for i, path := range result.Files {
		expected := filepath.Join(dir, filepath.Base(path))
		if path != expected {
			t.Fatalf("unexpected image path %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read png %q: %v", path, err)
		}
		if len(data) == 0 || data[0] != 0x89 || data[1] != 'P' {
			t.Fatalf("expected png header in %q", path)
		}

		metaPath := result.MetadataFiles[i]
		if filepath.Dir(metaPath) != dir {
			t.Fatalf("metadata path should be inside temp dir, got %q", metaPath)
		}
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatalf("read metadata %q: %v", metaPath, err)
		}
		var decoded Metadata
		if err := json.Unmarshal(metaBytes, &decoded); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
		if decoded.ImagePath == "" {
			t.Fatalf("metadata should include image path")
		}
		if decoded.Width != provider.frames[i].Metadata.Width {
			t.Fatalf("unexpected width in metadata: %d", decoded.Width)
		}
	}
}

func TestSchedulerCaptureCancellation(t *testing.T) {
	scheduler, err := NewScheduler(Options{Interval: time.Second, MaxPerMinute: 2, Provider: &fakeProvider{}})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := scheduler.Capture(ctx, t.TempDir()); err == nil {
		t.Fatalf("expected cancellation error")
	}
}
