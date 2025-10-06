package cmd

import (
	"bytes"
	"flag"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/config"
	"github.com/offlinefirst/limitless-context/pkg/runmanifest"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunCommandPlanOnly(t *testing.T) {
	cfg := config.Default()
	cfg.Capture.Screenshots.IntervalSeconds = 1
	cfg.Capture.Screenshots.MaxPerMinute = 1
	ctx := &AppContext{Config: cfg, Logger: newTestLogger()}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.Bool("plan-only", false, "")
	if err := fs.Parse([]string{"-plan-only"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	var stdout bytes.Buffer
	if err := runCapture(fs, nil, ctx, &stdout, io.Discard); err != nil {
		t.Fatalf("runCapture returned error: %v", err)
	}

	if !bytes.Contains(stdout.Bytes(), []byte("Resolved configuration")) {
		t.Fatalf("expected plan output, got %q", stdout.String())
	}
}

func TestRunCommandPreparesLayout(t *testing.T) {
	cfg := config.Default()
	cfg.Capture.Screenshots.IntervalSeconds = 1
	cfg.Capture.Screenshots.MaxPerMinute = 1
	runsDir := t.TempDir()
	cfg.Paths.RunsDir = runsDir
	ctx := &AppContext{Config: cfg, Logger: newTestLogger()}

	now := time.Date(2024, 5, 12, 9, 30, 0, 0, time.UTC)
	origTime := timeNow
	timeNow = func() time.Time { return now }
	defer func() { timeNow = origTime }()

	origHost := hostname
	hostname = func() (string, error) { return "test-host", nil }
	defer func() { hostname = origHost }()

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.Bool("plan-only", false, "")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	var stdout bytes.Buffer
	if err := runCapture(fs, nil, ctx, &stdout, io.Discard); err != nil {
		t.Fatalf("runCapture returned error: %v", err)
	}

	expectedID := now.Format("20060102_150405")
	layout := runmanifest.BuildLayout(runsDir, expectedID)

	man, err := runmanifest.Load(layout.ManifestPath)
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	if man.Status.State != "completed" {
		t.Fatalf("expected completed state, got %q", man.Status.State)
	}
	if man.Status.Termination == "" {
		t.Fatalf("expected termination reason recorded")
	}
	if man.Status.StartedAt == nil || man.Status.EndedAt == nil {
		t.Fatalf("expected lifecycle timestamps in manifest")
	}

	for _, dir := range []string{layout.VideoDir, layout.EventsDir, layout.ScreensDir, layout.ASRDir, layout.OCRDir, layout.BundlesDir, layout.ReportDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s: %v", dir, err)
		}
	}

	if !bytes.Contains(stdout.Bytes(), []byte("Prepared run directory")) {
		t.Fatalf("expected preparation output, got %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Event tap: 4 fine events")) {
		t.Fatalf("expected event tap summary, got %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("ASR:")) {
		t.Fatalf("expected ASR summary, got %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("OCR:")) {
		t.Fatalf("expected OCR summary, got %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Lifecycle:")) {
		t.Fatalf("expected lifecycle summary in output, got %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Controller timeline")) {
		t.Fatalf("expected controller timeline in output, got %q", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Subsystem status summary")) {
		t.Fatalf("expected subsystem status summary in output, got %q", stdout.String())
	}

	if len(man.Status.Subsystems) == 0 {
		t.Fatalf("expected subsystem statuses persisted to manifest")
	}
	if len(man.Status.Controller) == 0 {
		t.Fatalf("expected controller timeline persisted to manifest")
	}
}
