package runmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/config"
)

// SchemaVersion captures the manifest version for compatibility checks.
const SchemaVersion = 1

// Layout represents the absolute filesystem locations for a run.
type Layout struct {
	Root           string
	ManifestPath   string
	CaptureLogPath string
	VideoDir       string
	EventsDir      string
	ScreensDir     string
	ASRDir         string
	OCRDir         string
	BundlesDir     string
	ReportDir      string
}

// Paths holds the relative locations stored in the manifest for portability.
type Paths struct {
	Root        string `json:"root"`
	Manifest    string `json:"manifest"`
	CaptureLog  string `json:"capture_log"`
	Video       string `json:"video"`
	Events      string `json:"events"`
	Screenshots string `json:"screenshots"`
	ASR         string `json:"asr"`
	OCR         string `json:"ocr"`
	Bundles     string `json:"bundles"`
	Report      string `json:"report"`
}

// CaptureSettings records which subsystems are active for the run.
type CaptureSettings struct {
	DurationMinutes    int  `json:"duration_minutes"`
	VideoEnabled       bool `json:"video_enabled"`
	ScreenshotsEnabled bool `json:"screenshots_enabled"`
	EventsEnabled      bool `json:"events_enabled"`
	ASREnabled         bool `json:"asr_enabled"`
	OCREnabled         bool `json:"ocr_enabled"`
}

// Status summarises the lifecycle of a capture run.
type Status struct {
	State       string                    `json:"state"`
	Summary     string                    `json:"summary,omitempty"`
	StartedAt   *time.Time                `json:"started_at,omitempty"`
	EndedAt     *time.Time                `json:"ended_at,omitempty"`
	Termination string                    `json:"termination,omitempty"`
	Controller  []ControllerTimelineEntry `json:"controller_timeline,omitempty"`
	Subsystems  []SubsystemStatus         `json:"subsystems,omitempty"`
}

// ControllerTimelineEntry records controller state transitions for diagnostics.
type ControllerTimelineEntry struct {
	State     string    `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SubsystemStatus captures availability and outcome details for a subsystem.
type SubsystemStatus struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Available  bool   `json:"available"`
	State      string `json:"state"`
	Provider   string `json:"provider,omitempty"`
	Permission string `json:"permission,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Subsystem outcome states used in manifests for downstream tooling.
const (
	SubsystemStatePending     = "pending"
	SubsystemStateCompleted   = "completed"
	SubsystemStateSkipped     = "skipped"
	SubsystemStateUnavailable = "unavailable"
	SubsystemStateErrored     = "error"
)

// Manifest is the durable metadata describing a capture run.
type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	RunID         string          `json:"run_id"`
	CreatedAt     time.Time       `json:"created_at"`
	Hostname      string          `json:"hostname"`
	AppVersion    string          `json:"app_version"`
	ConfigSource  string          `json:"config_source"`
	Capture       CaptureSettings `json:"capture"`
	Paths         Paths           `json:"paths"`
	Status        Status          `json:"status"`
}

// Options captures the knobs for creating a new manifest.
type Options struct {
	RunID      string
	CreatedAt  time.Time
	Hostname   string
	AppVersion string
	Config     config.Config
	Layout     Layout
}

// New constructs a manifest using the supplied options.
func New(opts Options) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		RunID:         opts.RunID,
		CreatedAt:     opts.CreatedAt.UTC(),
		Hostname:      opts.Hostname,
		AppVersion:    opts.AppVersion,
		ConfigSource:  opts.Config.Source,
		Capture: CaptureSettings{
			DurationMinutes:    opts.Config.Capture.DurationMinutes,
			VideoEnabled:       opts.Config.Capture.VideoEnabled,
			ScreenshotsEnabled: opts.Config.Capture.ScreenshotsEnabled,
			EventsEnabled:      opts.Config.Capture.EventsEnabled,
			ASREnabled:         opts.Config.Capture.ASREnabled,
			OCREnabled:         opts.Config.Capture.OCREnabled,
		},
		Paths:  opts.Layout.RelativePaths(),
		Status: Status{State: "pending"},
	}
}

// BuildLayout creates an absolute filesystem layout for a run.
func BuildLayout(runsDir, runID string) Layout {
	root := filepath.Join(runsDir, runID)
	return Layout{
		Root:           root,
		ManifestPath:   filepath.Join(root, "manifest.json"),
		CaptureLogPath: filepath.Join(root, "capture.log"),
		VideoDir:       filepath.Join(root, "video"),
		EventsDir:      filepath.Join(root, "events"),
		ScreensDir:     filepath.Join(root, "screenshots"),
		ASRDir:         filepath.Join(root, "asr"),
		OCRDir:         filepath.Join(root, "ocr"),
		BundlesDir:     filepath.Join(root, "bundles"),
		ReportDir:      filepath.Join(root, "report"),
	}
}

// RelativePaths exposes the manifest-friendly relative paths for the layout.
func (l Layout) RelativePaths() Paths {
	return Paths{
		Root:        ".",
		Manifest:    filepath.Base(l.ManifestPath),
		CaptureLog:  filepath.Base(l.CaptureLogPath),
		Video:       filepath.Base(l.VideoDir),
		Events:      filepath.Base(l.EventsDir),
		Screenshots: filepath.Base(l.ScreensDir),
		ASR:         filepath.Base(l.ASRDir),
		OCR:         filepath.Base(l.OCRDir),
		Bundles:     filepath.Base(l.BundlesDir),
		Report:      filepath.Base(l.ReportDir),
	}
}

// EnsureFilesystem prepares the directory tree for a run layout.
func EnsureFilesystem(layout Layout) error {
	if err := os.MkdirAll(layout.Root, 0o755); err != nil {
		return fmt.Errorf("create run root: %w", err)
	}

	dirs := []string{
		layout.VideoDir,
		layout.EventsDir,
		layout.ScreensDir,
		layout.ASRDir,
		layout.OCRDir,
		layout.BundlesDir,
		layout.ReportDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	file, err := os.OpenFile(layout.CaptureLogPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("initialise capture log: %w", err)
	}
	defer file.Close()

	return nil
}

// Save writes the manifest JSON to disk with indentation for readability.
func Save(man Manifest, path string) error {
	data, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// Load reads a manifest JSON file from disk.
func Load(path string) (Manifest, error) {
	var man Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return man, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(data, &man); err != nil {
		return man, fmt.Errorf("decode manifest: %w", err)
	}
	return man, nil
}

// ResolveRunID chooses a run identifier derived from the timestamp and avoids collisions.
func ResolveRunID(runsDir string, now time.Time) (string, error) {
	if strings.TrimSpace(runsDir) == "" {
		return "", errors.New("runs directory must not be empty")
	}

	base := now.UTC().Format("20060102_150405")
	candidate := base
	suffix := 1
	for {
		_, err := os.Stat(filepath.Join(runsDir, candidate))
		if err == nil {
			candidate = fmt.Sprintf("%s_%02d", base, suffix)
			suffix++
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("inspect runs directory: %w", err)
		}
	}
}
