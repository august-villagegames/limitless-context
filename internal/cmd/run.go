package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/offlinefirst/limitless-context/internal/buildinfo"
	"github.com/offlinefirst/limitless-context/pkg/capture"
	"github.com/offlinefirst/limitless-context/pkg/runmanifest"
)

func newRunCommand() command {
	return command{
		name:        "run",
		description: "Start an offline capture session",
		configure: func(fs *flag.FlagSet) {
			fs.Bool("plan-only", false, "Print the resolved configuration without starting capture")
		},
		run: runCapture,
	}
}

var (
	timeNow      = time.Now
	hostname     = os.Hostname
	manifestSave = runmanifest.Save
)

func runCapture(fs *flag.FlagSet, args []string, ctx *AppContext, stdout io.Writer, stderr io.Writer) error {
	if ctx == nil {
		return fmt.Errorf("application context unavailable")
	}

	planOnly := boolFlag(fs, "plan-only")
	ctx.Logger.Info("run command invoked", "plan_only", planOnly, "runs_dir", ctx.Config.Paths.RunsDir, "config_source", ctx.Config.Source)

	if planOnly {
		printRunPlan(ctx, stdout)
		return nil
	}

	if err := os.MkdirAll(ctx.Config.Paths.RunsDir, 0o755); err != nil {
		return fmt.Errorf("ensure runs directory: %w", err)
	}

	runID, err := runmanifest.ResolveRunID(ctx.Config.Paths.RunsDir, timeNow())
	if err != nil {
		return fmt.Errorf("resolve run id: %w", err)
	}

	layout := runmanifest.BuildLayout(ctx.Config.Paths.RunsDir, runID)
	if err := runmanifest.EnsureFilesystem(layout); err != nil {
		return fmt.Errorf("prepare run filesystem: %w", err)
	}

	host, err := hostname()
	if err != nil {
		host = "unknown"
	}

	manifest := runmanifest.New(runmanifest.Options{
		RunID:      runID,
		CreatedAt:  timeNow(),
		Hostname:   host,
		AppVersion: buildinfo.Version(),
		Config:     ctx.Config,
		Layout:     layout,
	})

	if err := manifestSave(manifest, layout.ManifestPath); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	manifest.Status.State = "running"
	manifest.Status.Summary = "capture in progress"
	if err := manifestSave(manifest, layout.ManifestPath); err != nil {
		return fmt.Errorf("update manifest status: %w", err)
	}

	summary, err := capture.Run(context.Background(), capture.Options{
		Config: ctx.Config,
		Layout: layout,
		Logger: ctx.Logger,
		Clock:  timeNow,
	})

	if summary.Lifecycle != nil {
		started := summary.Lifecycle.StartedAt.UTC()
		finished := summary.Lifecycle.FinishedAt.UTC()
		manifest.Status.StartedAt = &started
		manifest.Status.EndedAt = &finished
		manifest.Status.Termination = summary.Lifecycle.TerminationCause
		if len(summary.Lifecycle.ControllerTimeline) > 0 {
			manifest.Status.Controller = append([]runmanifest.ControllerTimelineEntry(nil), summary.Lifecycle.ControllerTimeline...)
		}
	}
	if len(summary.Subsystems) > 0 {
		manifest.Status.Subsystems = append([]runmanifest.SubsystemStatus(nil), summary.Subsystems...)
	}

	if err != nil {
		manifest.Status.State = "failed"
		manifest.Status.Summary = err.Error()
		if manifest.Status.Termination == "" {
			manifest.Status.Termination = "error"
		}
		if ctx.Logger != nil {
			ctx.Logger.Error("capture run failed", "error", err)
		}
		if saveErr := manifestSave(manifest, layout.ManifestPath); saveErr != nil {
			return fmt.Errorf("run capture subsystems: %v (additionally failed to persist manifest: %w)", err, saveErr)
		}
		return fmt.Errorf("run capture subsystems: %w", err)
	}

	if manifest.Status.Termination == "" {
		manifest.Status.Termination = "completed"
	}
	manifest.Status.State = "completed"
	manifest.Status.Summary = fmt.Sprintf("capture finished (%s)", manifest.Status.Termination)
	if err := manifestSave(manifest, layout.ManifestPath); err != nil {
		return fmt.Errorf("finalise manifest: %w", err)
	}

	fmt.Fprintf(stdout, "Prepared run directory: %s\n", layout.Root)
	fmt.Fprintf(stdout, "Manifest: %s\n", layout.ManifestPath)
	fmt.Fprintf(stdout, "Capture log: %s\n", layout.CaptureLogPath)
	fmt.Fprintf(stdout, "Subsystem directories:\n")
	fmt.Fprintf(stdout, "  video: %s\n", layout.VideoDir)
	fmt.Fprintf(stdout, "  events: %s\n", layout.EventsDir)
	fmt.Fprintf(stdout, "  screenshots: %s\n", layout.ScreensDir)
	fmt.Fprintf(stdout, "  asr: %s\n", layout.ASRDir)
	fmt.Fprintf(stdout, "  ocr: %s\n", layout.OCRDir)
	fmt.Fprintf(stdout, "  bundles: %s\n", layout.BundlesDir)
	fmt.Fprintf(stdout, "  report: %s\n", layout.ReportDir)

	if len(summary.Subsystems) > 0 {
		fmt.Fprintf(stdout, "Subsystem status summary:\n")
		for _, subsystem := range summary.Subsystems {
			fmt.Fprintf(stdout, "  - %s: state=%s enabled=%t available=%t", subsystem.Name, subsystem.State, subsystem.Enabled, subsystem.Available)
			if subsystem.Provider != "" {
				fmt.Fprintf(stdout, " provider=%s", subsystem.Provider)
			}
			if subsystem.Permission != "" {
				fmt.Fprintf(stdout, " permission=%s", subsystem.Permission)
			}
			if subsystem.Message != "" {
				fmt.Fprintf(stdout, " (%s)", subsystem.Message)
			}
			fmt.Fprintln(stdout)
		}
	}

	if summary.Events != nil {
		fmt.Fprintf(stdout, "Event tap: %d fine events (%d buckets, %d filtered) -> %s\n", summary.Events.EventCount, summary.Events.BucketCount, summary.Events.FilteredCount, summary.Events.FinePath)
		fmt.Fprintf(stdout, "  coarse summary: %s\n", summary.Events.CoarsePath)
	} else {
		fmt.Fprintln(stdout, "Event tap: disabled via config")
	}

	if summary.Screenshots != nil {
		fmt.Fprintf(stdout, "Screenshots: %d captured, first at %s\n", summary.Screenshots.Count, summary.Screenshots.FirstCapture.Format(time.RFC3339))
	} else {
		fmt.Fprintln(stdout, "Screenshots: disabled via config")
	}

	if summary.Video != nil {
		fmt.Fprintf(stdout, "Video: segment recorded -> %s\n", summary.Video.File)
	} else {
		fmt.Fprintln(stdout, "Video: disabled via config")
	}

	if summary.ASR != nil {
		switch {
		case summary.ASR.MeetingDetected && summary.ASR.TranscriptPath != "":
			fmt.Fprintf(stdout, "ASR: meeting detected (%d segments) -> %s\n", summary.ASR.SegmentCount, summary.ASR.TranscriptPath)
		case summary.ASR.MeetingDetected:
			fmt.Fprintf(stdout, "ASR: meeting detected but Whisper unavailable (see %s)\n", summary.ASR.StatusPath)
		default:
			fmt.Fprintf(stdout, "ASR: no meeting detected (status: %s)\n", summary.ASR.StatusPath)
		}
	} else {
		fmt.Fprintln(stdout, "ASR: disabled via config")
	}

	if summary.OCR != nil {
		target := summary.OCR.IndexPath
		if target == "" {
			target = summary.OCR.StatusPath
		}
		fmt.Fprintf(stdout, "OCR: %d processed (%d skipped) -> %s\n", summary.OCR.ProcessedCount, summary.OCR.SkippedCount, target)
	} else {
		fmt.Fprintln(stdout, "OCR: disabled via config")
	}

	if summary.Lifecycle != nil {
		fmt.Fprintf(stdout, "Lifecycle: started %s, ended %s (termination: %s)\n", summary.Lifecycle.StartedAt.Format(time.RFC3339), summary.Lifecycle.FinishedAt.Format(time.RFC3339), summary.Lifecycle.TerminationCause)
		if len(summary.Lifecycle.ControllerTimeline) > 0 {
			fmt.Fprintf(stdout, "  Controller timeline:\n")
			for _, entry := range summary.Lifecycle.ControllerTimeline {
				fmt.Fprintf(stdout, "    - %s -> %s", entry.Timestamp.Format(time.RFC3339), entry.State)
				if entry.Reason != "" {
					fmt.Fprintf(stdout, " (%s)", entry.Reason)
				}
				fmt.Fprintln(stdout)
			}
		}
	}

	return nil
}

func printRunPlan(ctx *AppContext, stdout io.Writer) {
	fmt.Fprintf(stdout, "Resolved configuration (source: %s)\n", ctx.Config.Source)
	fmt.Fprintf(stdout, "  runs_dir: %s\n", ctx.Config.Paths.RunsDir)
	fmt.Fprintf(stdout, "  cache_dir: %s\n", ctx.Config.Paths.CacheDir)
	fmt.Fprintf(stdout, "  capture.video_enabled: %t\n", ctx.Config.Capture.VideoEnabled)
	fmt.Fprintf(stdout, "  capture.screenshots_enabled: %t\n", ctx.Config.Capture.ScreenshotsEnabled)
	fmt.Fprintf(stdout, "  capture.events_enabled: %t\n", ctx.Config.Capture.EventsEnabled)
	fmt.Fprintf(stdout, "  capture.asr_enabled: %t\n", ctx.Config.Capture.ASREnabled)
	fmt.Fprintf(stdout, "  capture.ocr_enabled: %t\n", ctx.Config.Capture.OCREnabled)
	fmt.Fprintf(stdout, "  logging.level: %s\n", ctx.Config.Logging.Level)
	fmt.Fprintf(stdout, "  logging.format: %s\n", ctx.Config.Logging.Format)
}

func boolFlag(fs *flag.FlagSet, name string) bool {
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	value, err := strconv.ParseBool(f.Value.String())
	if err != nil {
		return false
	}
	return value
}
