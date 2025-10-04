package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/config"
	"github.com/offlinefirst/limitless-context/pkg/events"
	"github.com/offlinefirst/limitless-context/pkg/runmanifest"
	"github.com/offlinefirst/limitless-context/pkg/screenshots"
	"github.com/offlinefirst/limitless-context/pkg/video"
)

// Options controls capture orchestration.
type Options struct {
	Config config.Config
	Layout runmanifest.Layout
	Logger *slog.Logger
	Clock  func() time.Time
}

// Summary reports the results of enabled subsystems.
type Summary struct {
	Events      *events.Result
	Screenshots *screenshots.Result
	Video       *video.Result
}

// Run executes the configured capture subsystems sequentially.
func Run(ctx context.Context, opts Options) (Summary, error) {
	if opts.Logger == nil {
		return Summary{}, errors.New("logger must be provided")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	logFile, err := os.OpenFile(opts.Layout.CaptureLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Summary{}, fmt.Errorf("open capture log: %w", err)
	}
	defer logFile.Close()

	summary := Summary{}

	if opts.Config.Capture.EventsEnabled {
		opts.Logger.Info("starting event tap capture")
		redactor, err := events.NewRedactor(opts.Config.Capture.Events.RedactEmails, opts.Config.Capture.Events.RedactPatterns)
		if err != nil {
			return Summary{}, fmt.Errorf("initialise event redactor: %w", err)
		}
		tap, err := events.NewTap(events.Options{
			FineInterval:   time.Duration(opts.Config.Capture.Events.FineIntervalSeconds) * time.Second,
			CoarseInterval: time.Duration(opts.Config.Capture.Events.CoarseIntervalSeconds) * time.Second,
			Redactor:       redactor,
			Clock:          clock,
		})
		if err != nil {
			return Summary{}, fmt.Errorf("initialise event tap: %w", err)
		}
		res, err := tap.Capture(ctx, opts.Layout.EventsDir)
		if err != nil {
			return Summary{}, fmt.Errorf("event capture failed: %w", err)
		}
		summary.Events = &res
		writeCaptureLog(logFile, clock(), "events", "captured %d fine events (%d buckets)", res.EventCount, res.BucketCount)
		opts.Logger.Info("event tap complete", "events", res.EventCount, "buckets", res.BucketCount)
	} else {
		writeCaptureLog(logFile, clock(), "events", "skipped (disabled in config)")
		opts.Logger.Info("event tap disabled via config")
	}

	if opts.Config.Capture.ScreenshotsEnabled {
		opts.Logger.Info("starting screenshot capture")
		scheduler, err := screenshots.NewScheduler(screenshots.Options{
			Interval:     time.Duration(opts.Config.Capture.Screenshots.IntervalSeconds) * time.Second,
			MaxPerMinute: opts.Config.Capture.Screenshots.MaxPerMinute,
			Clock:        clock,
		})
		if err != nil {
			return Summary{}, fmt.Errorf("initialise screenshot scheduler: %w", err)
		}
		res, err := scheduler.Capture(ctx, opts.Layout.ScreensDir)
		if err != nil {
			return Summary{}, fmt.Errorf("screenshot capture failed: %w", err)
		}
		summary.Screenshots = &res
		writeCaptureLog(logFile, clock(), "screenshots", "captured %d screenshots", res.Count)
		opts.Logger.Info("screenshot capture complete", "count", res.Count)
	} else {
		writeCaptureLog(logFile, clock(), "screenshots", "skipped (disabled in config)")
		opts.Logger.Info("screenshot capture disabled via config")
	}

	if opts.Config.Capture.VideoEnabled {
		opts.Logger.Info("starting video capture")
		recorder, err := video.NewRecorder(video.Options{
			ChunkSeconds: opts.Config.Capture.Video.ChunkSeconds,
			Format:       opts.Config.Capture.Video.Format,
			Clock:        clock,
		})
		if err != nil {
			return Summary{}, fmt.Errorf("initialise video recorder: %w", err)
		}
		res, err := recorder.Record(ctx, opts.Layout.VideoDir)
		if err != nil {
			return Summary{}, fmt.Errorf("video capture failed: %w", err)
		}
		summary.Video = &res
		writeCaptureLog(logFile, clock(), "video", "captured segment %s", res.File)
		opts.Logger.Info("video capture complete", "file", res.File)
	} else {
		writeCaptureLog(logFile, clock(), "video", "skipped (disabled in config)")
		opts.Logger.Info("video capture disabled via config")
	}

	return summary, nil
}

func writeCaptureLog(file *os.File, timestamp time.Time, subsystem, message string, args ...any) {
	if file == nil {
		return
	}
	formatted := message
	if len(args) > 0 {
		formatted = fmt.Sprintf(message, args...)
	}
	line := fmt.Sprintf("[%s] subsystem=%s %s\n", timestamp.UTC().Format(time.RFC3339), subsystem, formatted)
	_, _ = file.WriteString(line)
}
