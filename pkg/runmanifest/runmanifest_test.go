package runmanifest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/config"
)

func TestBuildLayoutAndRelativePaths(t *testing.T) {
	layout := BuildLayout("/tmp/runs", "20240512_093000")

	if layout.Root != filepath.Join("/tmp/runs", "20240512_093000") {
		t.Fatalf("unexpected root: %s", layout.Root)
	}

	rel := layout.RelativePaths()
	if rel.Root != "." {
		t.Fatalf("expected relative root '.', got %q", rel.Root)
	}
	if rel.Manifest != "manifest.json" {
		t.Fatalf("expected manifest.json, got %s", rel.Manifest)
	}
	if rel.Video != "video" {
		t.Fatalf("expected video directory name, got %s", rel.Video)
	}
}

func TestEnsureFilesystemCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	layout := BuildLayout(dir, "run")

	if err := EnsureFilesystem(layout); err != nil {
		t.Fatalf("EnsureFilesystem failed: %v", err)
	}

	paths := []string{
		layout.Root,
		layout.VideoDir,
		layout.EventsDir,
		layout.ScreensDir,
		layout.ASRDir,
		layout.OCRDir,
		layout.BundlesDir,
		layout.ReportDir,
	}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected path %s: %v", p, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected directory at %s", p)
		}
	}

	if _, err := os.Stat(layout.CaptureLogPath); err != nil {
		t.Fatalf("expected capture log file: %v", err)
	}
}

func TestNewManifest(t *testing.T) {
	cfg := config.Default()
	cfg.Source = "config.yaml"
	layout := BuildLayout("/tmp/runs", "run")
	now := time.Date(2024, 5, 12, 9, 30, 0, 0, time.UTC)

	man := New(Options{
		RunID:      "run",
		CreatedAt:  now,
		Hostname:   "host",
		AppVersion: "test",
		Config:     cfg,
		Layout:     layout,
	})

	if man.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected schema version: %d", man.SchemaVersion)
	}
	if man.CreatedAt.Location() != time.UTC {
		t.Fatalf("expected CreatedAt in UTC, got %s", man.CreatedAt.Location())
	}
	if man.Capture.VideoEnabled != cfg.Capture.VideoEnabled {
		t.Fatalf("capture mismatch for video")
	}
	if man.Paths.Manifest != "manifest.json" {
		t.Fatalf("unexpected manifest path: %s", man.Paths.Manifest)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	layout := BuildLayout(dir, "run")
	cfg := config.Default()
	cfg.Source = "explicit"
	now := time.Now().UTC().Round(time.Second)

	man := New(Options{
		RunID:      "run",
		CreatedAt:  now,
		Hostname:   "host",
		AppVersion: "version",
		Config:     cfg,
		Layout:     layout,
	})

	path := filepath.Join(dir, "manifest.json")
	if err := Save(man, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.RunID != man.RunID {
		t.Fatalf("expected RunID %s, got %s", man.RunID, loaded.RunID)
	}
	if loaded.Capture.EventsEnabled != man.Capture.EventsEnabled {
		t.Fatalf("expected EventsEnabled %t, got %t", man.Capture.EventsEnabled, loaded.Capture.EventsEnabled)
	}
}

func TestResolveRunID(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2024, 5, 12, 9, 30, 0, 0, time.UTC)

	if err := os.MkdirAll(filepath.Join(dir, now.Format("20060102_150405")), 0o755); err != nil {
		t.Fatalf("prep existing run: %v", err)
	}

	id, err := ResolveRunID(dir, now)
	if err != nil {
		t.Fatalf("ResolveRunID failed: %v", err)
	}
	expected := now.Format("20060102_150405") + "_01"
	if id != expected {
		t.Fatalf("expected %s, got %s", expected, id)
	}
}

func TestResolveRunIDEmptyRunsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path validation differs on windows")
	}
	if _, err := ResolveRunID(" ", time.Now()); err == nil {
		t.Fatalf("expected error for empty runs dir")
	}
}
