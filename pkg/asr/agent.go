package asr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/events"
)

// Options configure the ASR agent stub.
type Options struct {
	MeetingKeywords []string
	WindowTitles    []string
	WhisperBinary   string
	Language        string
	Clock           func() time.Time
	Redactor        events.Redactor
	LookPath        func(string) (string, error)
}

// Agent orchestrates meeting detection and transcript generation.
type Agent struct {
	keywords []string
	windows  []string
	whisper  string
	language string
	clock    func() time.Time
	redactor events.Redactor
	lookPath func(string) (string, error)
}

// Result summarises ASR output.
type Result struct {
	MeetingDetected  bool
	WhisperAvailable bool
	TranscriptPath   string
	StatusPath       string
	SegmentCount     int
	GuidancePath     string
}

type statusDocument struct {
	GeneratedAt      time.Time `json:"generated_at"`
	MeetingDetected  bool      `json:"meeting_detected"`
	WhisperAvailable bool      `json:"whisper_available"`
	Transcript       string    `json:"transcript,omitempty"`
	Guidance         string    `json:"guidance,omitempty"`
	Language         string    `json:"language"`
	Notes            []string  `json:"notes,omitempty"`
}

// NewAgent validates options and returns an agent.
func NewAgent(opts Options) (*Agent, error) {
	if len(opts.MeetingKeywords) == 0 {
		return nil, errors.New("meeting keywords must not be empty")
	}
	if len(opts.WindowTitles) == 0 {
		return nil, errors.New("window titles must not be empty")
	}
	whisper := strings.TrimSpace(opts.WhisperBinary)
	if whisper == "" {
		whisper = "whisper"
	}
	language := strings.TrimSpace(opts.Language)
	if language == "" {
		language = "en"
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	keywords := make([]string, 0, len(opts.MeetingKeywords))
	for _, kw := range opts.MeetingKeywords {
		trimmed := strings.TrimSpace(kw)
		if trimmed == "" {
			continue
		}
		keywords = append(keywords, strings.ToLower(trimmed))
	}
	if len(keywords) == 0 {
		return nil, errors.New("no usable meeting keywords provided")
	}

	windows := make([]string, 0, len(opts.WindowTitles))
	for _, title := range opts.WindowTitles {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			continue
		}
		windows = append(windows, trimmed)
	}
	if len(windows) == 0 {
		return nil, errors.New("no usable window titles provided")
	}

	return &Agent{
		keywords: keywords,
		windows:  windows,
		whisper:  whisper,
		language: strings.ToLower(language),
		clock:    clock,
		redactor: opts.Redactor,
		lookPath: lookPath,
	}, nil
}

// Capture performs meeting detection and writes transcript fixtures.
func (a *Agent) Capture(ctx context.Context, destDir string) (Result, error) {
	if strings.TrimSpace(destDir) == "" {
		return Result{}, errors.New("destination directory must not be empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ensure asr directory: %w", err)
	}

	detected := a.detectMeeting()
	available := a.detectWhisper()

	statusPath := filepath.Join(destDir, "status.json")
	guidancePath := ""
	transcriptPath := ""
	segmentCount := 0

	notes := []string{}

	if detected && available {
		transcriptPath = filepath.Join(destDir, "meeting_0001.vtt")
		if err := a.writeTranscript(ctx, transcriptPath); err != nil {
			return Result{}, err
		}
		segmentCount = 2
	}

	if detected && !available {
		guidancePath = filepath.Join(destDir, "install_whisper.txt")
		msg := fmt.Sprintf("whisper binary %q not found in PATH", a.whisper)
		notes = append(notes, msg)
		guidance := "Install whisper.cpp locally and place the binary on PATH to enable offline ASR."
		if err := os.WriteFile(guidancePath, []byte(guidance+"\n"), 0o644); err != nil {
			return Result{}, fmt.Errorf("write guidance: %w", err)
		}
	}

	status := statusDocument{
		GeneratedAt:      a.clock().UTC(),
		MeetingDetected:  detected,
		WhisperAvailable: available,
		Transcript:       transcriptPath,
		Guidance:         guidancePath,
		Language:         a.language,
		Notes:            notes,
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("marshal asr status: %w", err)
	}
	if err := os.WriteFile(statusPath, data, 0o644); err != nil {
		return Result{}, fmt.Errorf("write asr status: %w", err)
	}

	return Result{
		MeetingDetected:  detected,
		WhisperAvailable: available,
		TranscriptPath:   transcriptPath,
		StatusPath:       statusPath,
		SegmentCount:     segmentCount,
		GuidancePath:     guidancePath,
	}, nil
}

func (a *Agent) detectMeeting() bool {
	for _, title := range a.windows {
		lower := strings.ToLower(title)
		for _, keyword := range a.keywords {
			if strings.Contains(lower, keyword) {
				return true
			}
		}
	}
	return false
}

func (a *Agent) detectWhisper() bool {
	if a.lookPath == nil {
		return false
	}
	_, err := a.lookPath(a.whisper)
	return err == nil
}

func (a *Agent) writeTranscript(ctx context.Context, path string) error {
	segments := []struct {
		Start string
		End   string
		Text  string
	}{
		{Start: "00:00:00.000", End: "00:00:05.000", Text: "Team sync kicks off with launch checklist."},
		{Start: "00:00:05.000", End: "00:00:10.000", Text: "Action item: send recap to owner@example.com before EOD."},
	}

	builder := strings.Builder{}
	builder.WriteString("WEBVTT\n\n")
	for i, segment := range segments {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		text := a.redactor.ApplyString(segment.Text)
		builder.WriteString(fmt.Sprintf("%d\n%s --> %s\n%s\n\n", i+1, segment.Start, segment.End, text))
	}

	return os.WriteFile(path, []byte(builder.String()), 0o644)
}
