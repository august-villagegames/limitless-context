package events

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewTapValidation(t *testing.T) {
	if _, err := NewTap(Options{FineInterval: 0, CoarseInterval: time.Second}); err == nil {
		t.Fatalf("expected error for zero fine interval")
	}
	if _, err := NewTap(Options{FineInterval: time.Second, CoarseInterval: 0}); err == nil {
		t.Fatalf("expected error for zero coarse interval")
	}
	if _, err := NewTap(Options{FineInterval: time.Second, CoarseInterval: time.Millisecond}); err == nil {
		t.Fatalf("expected error when coarse < fine")
	}
}

func TestTapCaptureProducesDualGranularity(t *testing.T) {
	redactor, err := NewRedactor(true, []string{`token=\w+`})
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	base := time.Date(2024, 3, 14, 9, 26, 0, 0, time.UTC)
	tap, err := NewTap(Options{
		FineInterval:   10 * time.Second,
		CoarseInterval: time.Minute,
		Redactor:       redactor,
		Clock: func() time.Time {
			return base
		},
	})
	if err != nil {
		t.Fatalf("new tap: %v", err)
	}

	dir := t.TempDir()
	result, err := tap.Capture(context.Background(), dir)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if result.EventCount != 4 {
		t.Fatalf("expected 4 events, got %d", result.EventCount)
	}
	if result.BucketCount == 0 {
		t.Fatalf("expected at least one coarse bucket")
	}

	finePath := filepath.Join(dir, "events_fine.jsonl")
	data, err := os.ReadFile(finePath)
	if err != nil {
		t.Fatalf("read fine events: %v", err)
	}
	if strings.Contains(string(data), "support@example.com") {
		t.Fatalf("expected email to be redacted in fine stream")
	}
	if strings.Contains(string(data), "token=abcd1234") {
		t.Fatalf("expected custom token to be redacted")
	}

	coarsePath := filepath.Join(dir, "events_coarse.json")
	coarseData, err := os.ReadFile(coarsePath)
	if err != nil {
		t.Fatalf("read coarse summary: %v", err)
	}

	var buckets []CoarseBucket
	if err := json.Unmarshal(coarseData, &buckets); err != nil {
		t.Fatalf("decode coarse summary: %v", err)
	}
	if len(buckets) == 0 {
		t.Fatalf("expected buckets to be recorded")
	}
	if buckets[0].Count == 0 {
		t.Fatalf("expected bucket to count events")
	}
}

func TestRedactorAppliesPatterns(t *testing.T) {
	redactor, err := NewRedactor(true, []string{`secret-\d+`})
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	input := "Email jane@example.com uses secret-123"
	if out := redactor.ApplyString(input); strings.Contains(out, "jane@example.com") {
		t.Fatalf("expected email to be redacted: %s", out)
	}
	if out := redactor.ApplyString(input); strings.Contains(out, "secret-123") {
		t.Fatalf("expected custom token to be redacted: %s", out)
	}

	metadata := map[string]string{"note": input}
	out := redactor.ApplyMetadata(metadata)
	if out["note"] == input {
		t.Fatalf("expected metadata value to be redacted")
	}
	if len(metadata) != len(out) {
		t.Fatalf("expected metadata clone to preserve size")
	}
}

func TestTapRespectsCancellation(t *testing.T) {
	redactor, err := NewRedactor(false, nil)
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	tap, err := NewTap(Options{FineInterval: time.Second, CoarseInterval: time.Second, Redactor: redactor})
	if err != nil {
		t.Fatalf("new tap: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := tap.Capture(ctx, t.TempDir()); err == nil {
		t.Fatalf("expected capture to respect cancellation")
	}
}
