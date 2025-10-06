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

type stubSource struct {
	events []Event
}

func (s stubSource) Stream(ctx context.Context, emit func(Event) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, event := range s.events {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}

type blockingSource struct{}

func (blockingSource) Stream(ctx context.Context, emit func(Event) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	return ctx.Err()
}

func fixtureTimeline(base time.Time, fine time.Duration) []Event {
	return []Event{
		{
			Timestamp: base,
			Category:  "keyboard",
			Action:    "press",
			Target:    "compose",
			Metadata: map[string]string{
				"text": "Drafting email to support@example.com about rollout",
				"app":  "mail",
				"url":  "mailto:support@example.com",
			},
		},
		{
			Timestamp: base.Add(fine),
			Category:  "mouse",
			Action:    "click",
			Target:    "submit-button",
			Metadata: map[string]string{
				"label": "Submit order",
				"app":   "checkout",
				"url":   "https://orders.example.com/checkout",
			},
		},
		{
			Timestamp: base.Add(2 * fine),
			Category:  "window",
			Action:    "focus",
			Target:    "docs-app",
			Metadata: map[string]string{
				"title": "Roadmap token=abcd1234",
				"app":   "docs",
				"url":   "https://docs.example.com/roadmap",
			},
		},
		{
			Timestamp: base.Add(3 * fine),
			Category:  "clipboard",
			Action:    "copy",
			Target:    "",
			Metadata: map[string]string{
				"preview": "Quarterly plan summary",
				"app":     "notes",
				"url":     "https://notes.example.com/q1",
			},
		},
	}
}

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
		Source: stubSource{events: fixtureTimeline(base, 10*time.Second)},
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
	if result.FilteredCount != 0 {
		t.Fatalf("expected no filtered events, got %d", result.FilteredCount)
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

func TestTapPrivacyFiltersEvents(t *testing.T) {
	redactor, err := NewRedactor(false, nil)
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	base := time.Now().UTC()
	tap, err := NewTap(Options{
		FineInterval:   time.Second,
		CoarseInterval: 2 * time.Second,
		Redactor:       redactor,
		Privacy:        NewPrivacyPolicy([]string{"docs"}, nil, true),
		Source:         stubSource{events: fixtureTimeline(base, time.Second)},
	})
	if err != nil {
		t.Fatalf("new tap: %v", err)
	}

	dir := t.TempDir()
	result, err := tap.Capture(context.Background(), dir)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if result.EventCount == 0 {
		t.Fatalf("expected at least one event to be allowed")
	}
	if result.FilteredCount == 0 {
		t.Fatalf("expected some events to be filtered")
	}

	data, err := os.ReadFile(filepath.Join(dir, "events_fine.jsonl"))
	if err != nil {
		t.Fatalf("read fine events: %v", err)
	}
	if strings.Contains(string(data), "mail\n") {
		t.Fatalf("expected events outside allow-list to be removed")
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

	tap, err := NewTap(Options{
		FineInterval:   time.Second,
		CoarseInterval: time.Second,
		Redactor:       redactor,
		Source:         blockingSource{},
	})
	if err != nil {
		t.Fatalf("new tap: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := tap.Capture(ctx, t.TempDir()); err == nil {
		t.Fatalf("expected capture to respect cancellation")
	}
}
