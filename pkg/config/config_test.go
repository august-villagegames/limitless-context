package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(cwd)

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Paths.RunsDir != "runs" {
		t.Fatalf("expected default runs dir, got %q", cfg.Paths.RunsDir)
	}
	if cfg.Source != "<defaults>" {
		t.Fatalf("expected default source marker, got %q", cfg.Source)
	}
	if cfg.Capture.DurationMinutes != 60 {
		t.Fatalf("unexpected default duration minutes: %d", cfg.Capture.DurationMinutes)
	}
	if cfg.Capture.Video.ChunkSeconds != 300 {
		t.Fatalf("unexpected default video chunk: %d", cfg.Capture.Video.ChunkSeconds)
	}
	if cfg.Capture.Events.FineIntervalSeconds != 10 {
		t.Fatalf("unexpected default events fine interval: %d", cfg.Capture.Events.FineIntervalSeconds)
	}
}

func TestLoadFromFileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "paths:\n  runs_dir: artifacts\n  cache_dir: .cache\ncapture:\n  duration_minutes: 45\n  video_enabled: false\n  video:\n    chunk_seconds: 120\n    format: mkv\n  screenshots_enabled: true\n  screenshots:\n    interval_seconds: 15\n    max_per_minute: 4\n  events_enabled: false\n  events:\n    fine_interval_seconds: 5\n    coarse_interval_seconds: 30\n    redact_emails: false\n    redact_patterns: password,token\nlogging:\n  level: DEBUG\n  format: console\n"

	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := cfg.Paths.RunsDir; got != "artifacts" {
		t.Fatalf("unexpected runs dir: %q", got)
	}
	if got := cfg.Paths.CacheDir; got != ".cache" {
		t.Fatalf("unexpected cache dir: %q", got)
	}
	if cfg.Capture.VideoEnabled {
		t.Fatalf("expected video capture disabled")
	}
	if cfg.Capture.EventsEnabled {
		t.Fatalf("expected events capture disabled")
	}
	if cfg.Capture.DurationMinutes != 45 {
		t.Fatalf("unexpected capture duration: %d", cfg.Capture.DurationMinutes)
	}
	if cfg.Capture.Video.ChunkSeconds != 120 {
		t.Fatalf("unexpected video chunk seconds: %d", cfg.Capture.Video.ChunkSeconds)
	}
	if cfg.Capture.Video.Format != "mkv" {
		t.Fatalf("unexpected video format: %q", cfg.Capture.Video.Format)
	}
	if cfg.Capture.Screenshots.IntervalSeconds != 15 {
		t.Fatalf("unexpected screenshot interval: %d", cfg.Capture.Screenshots.IntervalSeconds)
	}
	if cfg.Capture.Screenshots.MaxPerMinute != 4 {
		t.Fatalf("unexpected screenshot max per minute: %d", cfg.Capture.Screenshots.MaxPerMinute)
	}
	if cfg.Capture.Events.FineIntervalSeconds != 5 {
		t.Fatalf("unexpected events fine interval: %d", cfg.Capture.Events.FineIntervalSeconds)
	}
	if cfg.Capture.Events.CoarseIntervalSeconds != 30 {
		t.Fatalf("unexpected events coarse interval: %d", cfg.Capture.Events.CoarseIntervalSeconds)
	}
	if cfg.Capture.Events.RedactEmails {
		t.Fatalf("expected redact emails disabled")
	}
	if got := len(cfg.Capture.Events.RedactPatterns); got != 2 {
		t.Fatalf("expected two redact patterns, got %d", got)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("unexpected log level: %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "console" {
		t.Fatalf("unexpected log format: %q", cfg.Logging.Format)
	}
	if cfg.Source != cfgPath {
		t.Fatalf("expected source to equal path, got %q", cfg.Source)
	}
}

func TestUnknownKeyReturnsError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "capture:\n  unsupported: true\n"

	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil {
		t.Fatalf("expected error for unsupported key")
	}
}
