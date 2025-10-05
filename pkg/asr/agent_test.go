package asr

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/events"
)

func TestAgentCaptureProducesTranscript(t *testing.T) {
	dir := t.TempDir()
	redactor, err := events.NewRedactor(true, nil)
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	lookPath := func(string) (string, error) { return "/usr/local/bin/whisper", nil }

	agent, err := NewAgent(Options{
		MeetingKeywords: []string{"Zoom"},
		WindowTitles:    []string{"Weekly Sync - Zoom"},
		WhisperBinary:   "whisper",
		Language:        "en",
		Clock:           func() time.Time { return time.Date(2024, 5, 12, 9, 30, 0, 0, time.UTC) },
		Redactor:        redactor,
		LookPath:        lookPath,
	})
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}

	result, err := agent.Capture(context.Background(), dir)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if !result.MeetingDetected {
		t.Fatalf("expected meeting detection")
	}
	if !result.WhisperAvailable {
		t.Fatalf("expected whisper availability")
	}
	if result.TranscriptPath == "" {
		t.Fatalf("expected transcript path")
	}

	data, err := os.ReadFile(result.TranscriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(data), "owner@example.com") {
		t.Fatalf("expected transcript to be redacted: %s", string(data))
	}

	statusData, err := os.ReadFile(result.StatusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	var status statusDocument
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Transcript == "" {
		t.Fatalf("expected status to include transcript reference")
	}
}

func TestAgentCaptureWhenWhisperMissing(t *testing.T) {
	dir := t.TempDir()
	redactor, err := events.NewRedactor(false, nil)
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	lookPath := func(string) (string, error) { return "", os.ErrNotExist }

	agent, err := NewAgent(Options{
		MeetingKeywords: []string{"meet"},
		WindowTitles:    []string{"Design Review - Google Meet"},
		WhisperBinary:   "whisper",
		Redactor:        redactor,
		LookPath:        lookPath,
	})
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}

	result, err := agent.Capture(context.Background(), dir)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if !result.MeetingDetected {
		t.Fatalf("expected meeting detected")
	}
	if result.WhisperAvailable {
		t.Fatalf("expected whisper to be unavailable")
	}
	if result.TranscriptPath != "" {
		t.Fatalf("expected transcript to be empty when whisper missing")
	}
	if result.GuidancePath == "" {
		t.Fatalf("expected guidance file when whisper missing")
	}
	if _, err := os.Stat(result.GuidancePath); err != nil {
		t.Fatalf("guidance file missing: %v", err)
	}

	statusData, err := os.ReadFile(result.StatusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), "whisper binary") {
		t.Fatalf("expected status to mention missing whisper: %s", string(statusData))
	}
}
