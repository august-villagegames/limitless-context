package capture

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/config"
	"github.com/offlinefirst/limitless-context/pkg/runmanifest"
)

func TestRunExecutesEnabledSubsystems(t *testing.T) {
	cfg := config.Default()

	dir := t.TempDir()
	layout := runmanifest.BuildLayout(dir, "test")
	if err := runmanifest.EnsureFilesystem(layout); err != nil {
		t.Fatalf("ensure filesystem: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)

	summary, err := Run(context.Background(), Options{
		Config: cfg,
		Layout: layout,
		Logger: logger,
		Clock:  func() time.Time { return base },
	})
	if err != nil {
		t.Fatalf("run capture: %v", err)
	}

	if summary.Events == nil || summary.Screenshots == nil || summary.Video == nil {
		t.Fatalf("expected all subsystems to run: %#v", summary)
	}

	captureLog, err := os.ReadFile(layout.CaptureLogPath)
	if err != nil {
		t.Fatalf("read capture log: %v", err)
	}

	content := string(captureLog)
	for _, subsystem := range []string{"events", "screenshots", "video"} {
		if !strings.Contains(content, subsystem) {
			t.Fatalf("expected capture log to mention %s", subsystem)
		}
	}

	if _, err := os.Stat(filepath.Join(layout.EventsDir, "events_fine.jsonl")); err != nil {
		t.Fatalf("expected event fine stream: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.ScreensDir, "screenshot_001.txt")); err != nil {
		t.Fatalf("expected screenshot output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.VideoDir, "segment_0001.webm")); err != nil {
		t.Fatalf("expected video stub output: %v", err)
	}
}

func TestRunRespectsDisabledSubsystems(t *testing.T) {
	cfg := config.Default()
	cfg.Capture.EventsEnabled = false
	cfg.Capture.ScreenshotsEnabled = false
	cfg.Capture.VideoEnabled = false

	dir := t.TempDir()
	layout := runmanifest.BuildLayout(dir, "disabled")
	if err := runmanifest.EnsureFilesystem(layout); err != nil {
		t.Fatalf("ensure filesystem: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	summary, err := Run(context.Background(), Options{
		Config: cfg,
		Layout: layout,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("run capture: %v", err)
	}

	if summary.Events != nil || summary.Screenshots != nil || summary.Video != nil {
		t.Fatalf("expected no subsystems to run: %#v", summary)
	}

	captureLog, err := os.ReadFile(layout.CaptureLogPath)
	if err != nil {
		t.Fatalf("read capture log: %v", err)
	}
	content := string(captureLog)
	for _, phrase := range []string{"skipped (disabled in config)"} {
		if !strings.Contains(content, phrase) {
			t.Fatalf("expected log to contain %q", phrase)
		}
	}
}
