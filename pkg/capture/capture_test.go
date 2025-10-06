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
	cfg.Capture.Screenshots.IntervalSeconds = 1
	cfg.Capture.Screenshots.MaxPerMinute = 1

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

	if summary.Events == nil || summary.Screenshots == nil || summary.Video == nil || summary.ASR == nil || summary.OCR == nil {
		t.Fatalf("expected all subsystems to run: %#v", summary)
	}
	if summary.Events.FilteredCount != 0 {
		t.Fatalf("expected no filtered events in default run")
	}
	if !summary.ASR.MeetingDetected {
		t.Fatalf("expected meeting detection in ASR summary")
	}
	if summary.ASR.TranscriptPath == "" {
		if summary.ASR.WhisperAvailable {
			t.Fatalf("expected transcript when Whisper is available")
		}
		if summary.ASR.GuidancePath == "" {
			t.Fatalf("expected guidance path when transcript is absent")
		}
		if _, err := os.Stat(summary.ASR.GuidancePath); err != nil {
			t.Fatalf("expected ASR guidance file: %v", err)
		}
	} else {
		if _, err := os.Stat(summary.ASR.TranscriptPath); err != nil {
			t.Fatalf("expected ASR transcript file: %v", err)
		}
	}
	if summary.OCR.ProcessedCount == 0 {
		t.Fatalf("expected OCR to process screenshots")
	}
	if summary.Lifecycle == nil {
		t.Fatalf("expected lifecycle summary")
	}
	if summary.Lifecycle.StartedAt != base || summary.Lifecycle.FinishedAt != base {
		t.Fatalf("expected lifecycle timestamps to use test clock")
	}
	if summary.Lifecycle.TerminationCause != "completed" {
		t.Fatalf("expected completed termination cause, got %q", summary.Lifecycle.TerminationCause)
	}
	if len(summary.Lifecycle.ControllerTimeline) == 0 {
		t.Fatalf("expected controller timeline entries")
	}
	if len(summary.Subsystems) == 0 {
		t.Fatalf("expected subsystem status summaries")
	}

	captureLog, err := os.ReadFile(layout.CaptureLogPath)
	if err != nil {
		t.Fatalf("read capture log: %v", err)
	}

	content := string(captureLog)
	for _, subsystem := range []string{"events", "screenshots", "video", "asr", "ocr"} {
		if !strings.Contains(content, subsystem) {
			t.Fatalf("expected capture log to mention %s", subsystem)
		}
	}

	if _, err := os.Stat(filepath.Join(layout.EventsDir, "events_fine.jsonl")); err != nil {
		t.Fatalf("expected event fine stream: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.ScreensDir, "screenshot_001.png")); err != nil {
		t.Fatalf("expected screenshot png output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.ScreensDir, "screenshot_001.json")); err != nil {
		t.Fatalf("expected screenshot metadata output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.VideoDir, "segment_0001.webm")); err != nil {
		t.Fatalf("expected video stub output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.ASRDir, "status.json")); err != nil {
		t.Fatalf("expected ASR status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.OCRDir, "status.json")); err != nil {
		t.Fatalf("expected OCR status: %v", err)
	}
}

func TestRunRespectsDisabledSubsystems(t *testing.T) {
	cfg := config.Default()
	cfg.Capture.EventsEnabled = false
	cfg.Capture.ScreenshotsEnabled = false
	cfg.Capture.VideoEnabled = false
	cfg.Capture.ASREnabled = false
	cfg.Capture.OCREnabled = false

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

	if summary.Events != nil || summary.Screenshots != nil || summary.Video != nil || summary.ASR != nil || summary.OCR != nil {
		t.Fatalf("expected no subsystems to run: %#v", summary)
	}
	if summary.Lifecycle == nil {
		t.Fatalf("expected lifecycle summary when subsystems disabled")
	}
	if summary.Lifecycle.TerminationCause != "completed" {
		t.Fatalf("expected completed lifecycle cause, got %q", summary.Lifecycle.TerminationCause)
	}
	if len(summary.Lifecycle.ControllerTimeline) == 0 {
		t.Fatalf("expected controller timeline when subsystems disabled")
	}
	if len(summary.Subsystems) == 0 {
		t.Fatalf("expected subsystem status summaries when disabled")
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

func TestRunHonorsControllerPause(t *testing.T) {
	cfg := config.Default()
	cfg.Capture.Screenshots.IntervalSeconds = 1
	cfg.Capture.Screenshots.MaxPerMinute = 1
	dir := t.TempDir()
	layout := runmanifest.BuildLayout(dir, "control")
	if err := runmanifest.EnsureFilesystem(layout); err != nil {
		t.Fatalf("ensure filesystem: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controller := NewController()
	controller.Pause()

	done := make(chan struct{})
	go func() {
		_, err := Run(context.Background(), Options{Config: cfg, Layout: layout, Logger: logger, Control: controller})
		if err != nil {
			t.Errorf("run returned error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("expected run to block while paused")
	case <-time.After(150 * time.Millisecond):
	}

	controller.Resume()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("run did not resume after controller resume")
	}
}

func TestRunStopsWhenDurationElapsed(t *testing.T) {
	cfg := config.Default()
	dir := t.TempDir()
	layout := runmanifest.BuildLayout(dir, "duration")
	if err := runmanifest.EnsureFilesystem(layout); err != nil {
		t.Fatalf("ensure filesystem: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controller := NewController()
	controller.Kill(ErrDurationElapsed)

	summary, err := Run(context.Background(), Options{Config: cfg, Layout: layout, Logger: logger, Control: controller})
	if err != nil {
		t.Fatalf("expected nil error when duration elapsed, got %v", err)
	}
	if summary.Events != nil || summary.Screenshots != nil || summary.Video != nil {
		t.Fatalf("expected subsystems to be skipped when duration elapsed")
	}
	if summary.Lifecycle == nil || summary.Lifecycle.TerminationCause != "duration_elapsed" {
		t.Fatalf("expected duration_elapsed termination, got %#v", summary.Lifecycle)
	}
	if summary.Lifecycle != nil && len(summary.Lifecycle.ControllerTimeline) == 0 {
		t.Fatalf("expected controller timeline when duration elapsed")
	}
	if len(summary.Subsystems) == 0 {
		t.Fatalf("expected subsystem status summaries when duration elapsed")
	}
}
