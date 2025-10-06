package video

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRecorderValidation(t *testing.T) {
	if _, err := NewRecorder(Options{ChunkSeconds: 0, Format: "webm"}); err == nil {
		t.Fatalf("expected error for zero chunk duration")
	}
	if _, err := NewRecorder(Options{ChunkSeconds: 5, Format: ""}); err == nil {
		t.Fatalf("expected error for empty format")
	}
	if _, err := NewRecorder(Options{ChunkSeconds: 5, Format: "webm"}); err == nil {
		t.Fatalf("expected error for unsupported format")
	}
}

func TestRecorderWritesSegment(t *testing.T) {
	SetNativeFactory(func(format string) (NativeRecorder, error) {
		return &fakeNativeRecorder{format: format}, nil
	})
	t.Cleanup(func() { SetNativeFactory(nil) })

	base := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	recorder, err := NewRecorder(Options{ChunkSeconds: 120, Format: "mp4", Clock: func() time.Time { return base }})
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	dir := t.TempDir()
	result, err := recorder.Record(context.Background(), dir)
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	expected := filepath.Join(dir, "segment_20240201T120000.mp4")
	if result.File != expected {
		t.Fatalf("unexpected file path %q", result.File)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected video segment to exist: %v", err)
	}
	if result.Ended.Sub(result.Started) != 120*time.Second {
		t.Fatalf("unexpected duration: %s", result.Ended.Sub(result.Started))
	}
}

func TestRecorderCancellation(t *testing.T) {
	SetNativeFactory(func(format string) (NativeRecorder, error) {
		return &fakeNativeRecorder{format: format}, nil
	})
	t.Cleanup(func() { SetNativeFactory(nil) })

	recorder, err := NewRecorder(Options{ChunkSeconds: 60, Format: "mp4"})
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := recorder.Record(ctx, t.TempDir()); err == nil {
		t.Fatalf("expected cancellation error")
	}
}

type fakeNativeRecorder struct {
	format string
}

func (f *fakeNativeRecorder) Record(ctx context.Context, dest, filename string, started time.Time, duration time.Duration) (string, error) {
	if f.format != "mp4" {
		return "", fmt.Errorf("unexpected format %s", f.format)
	}
	if ctx != nil && ctx.Err() != nil {
		return "", ctx.Err()
	}
	path := filepath.Join(dest, filename)
	payload := fmt.Sprintf("fake segment from %s to %s\n", started.Format(time.RFC3339), started.Add(duration).Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func TestDetectEnvironment(t *testing.T) {
	env := DetectEnvironment()
	if env.Provider == "" {
		t.Fatalf("expected provider to be populated")
	}
	if env.Permission == "" {
		t.Fatalf("expected permission string for manifest integration")
	}
	if env.Message == "" {
		t.Fatalf("expected informative message from environment detection")
	}
}
