package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/asr"
	"github.com/offlinefirst/limitless-context/pkg/config"
	"github.com/offlinefirst/limitless-context/pkg/events"
	"github.com/offlinefirst/limitless-context/pkg/ocr"
	"github.com/offlinefirst/limitless-context/pkg/runmanifest"
	"github.com/offlinefirst/limitless-context/pkg/screenshots"
	"github.com/offlinefirst/limitless-context/pkg/video"
)

// ErrDurationElapsed signals that the configured capture duration was reached.
var ErrDurationElapsed = errors.New("capture duration elapsed")

// Options controls capture orchestration.
type Options struct {
	Config  config.Config
	Layout  runmanifest.Layout
	Logger  *slog.Logger
	Clock   func() time.Time
	Control *Controller
}

// Summary reports the results of enabled subsystems.
type Summary struct {
	Events      *events.Result
	Screenshots *screenshots.Result
	Video       *video.Result
	ASR         *asr.Result
	OCR         *ocr.Result
	Lifecycle   *LifecycleSummary
	Subsystems  []runmanifest.SubsystemStatus
}

// LifecycleSummary captures the coarse lifecycle timestamps for a run.
type LifecycleSummary struct {
	StartedAt          time.Time
	FinishedAt         time.Time
	TerminationCause   string
	ControllerTimeline []runmanifest.ControllerTimelineEntry
}

type subsystemRunner struct {
	name   string
	status runmanifest.SubsystemStatus
	run    func(context.Context) (string, func(*Summary), error)
}

// Run executes the configured capture subsystems concurrently while respecting controller signals.
func Run(ctx context.Context, opts Options) (summary Summary, err error) {
	if opts.Logger == nil {
		return Summary{}, errors.New("logger must be provided")
	}
	if ctx == nil {
		ctx = context.Background()
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

	controller := opts.Control
	if controller == nil {
		controller = NewController()
	}

	redactor, err := events.NewRedactor(opts.Config.Capture.Events.RedactEmails, opts.Config.Capture.Events.RedactPatterns)
	if err != nil {
		return Summary{}, fmt.Errorf("initialise event redactor: %w", err)
	}
	privacy := events.NewPrivacyPolicy(opts.Config.Capture.Privacy.AllowApps, opts.Config.Capture.Privacy.AllowURLs, opts.Config.Capture.Privacy.DropUnknown)

	start := clock()
	summary = Summary{
		Lifecycle: &LifecycleSummary{StartedAt: start, ControllerTimeline: make([]runmanifest.ControllerTimelineEntry, 0, 4)},
	}

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	var summaryMu sync.Mutex
	var logMu sync.Mutex
	logCapture := func(ts time.Time, subsystem, message string, args ...any) {
		logMu.Lock()
		writeCaptureLog(logFile, ts, subsystem, message, args...)
		logMu.Unlock()
	}

	stateCh, unsubscribe := controller.Subscribe()
	var controllerOnce sync.Once
	ready := make(chan struct{})
	go func() {
		var readyOnce sync.Once
		for change := range stateCh {
			timestamp := clock()
			logCapture(timestamp, "controller", "state=%s reason=%s", change.State, change.Reason)
			summaryMu.Lock()
			if summary.Lifecycle != nil {
				summary.Lifecycle.ControllerTimeline = append(summary.Lifecycle.ControllerTimeline, runmanifest.ControllerTimelineEntry{
					State:     change.State,
					Reason:    change.Reason,
					Timestamp: timestamp,
				})
			}
			summaryMu.Unlock()
			readyOnce.Do(func() { close(ready) })
			if change.State == "stopping" {
				controllerOnce.Do(func() { runCancel() })
			}
		}
		readyOnce.Do(func() { close(ready) })
	}()
	<-ready
	defer unsubscribe()

	duration := time.Duration(opts.Config.Capture.DurationMinutes) * time.Minute
	timerCtx, timerCancel := context.WithCancel(context.Background())
	var countdown sync.WaitGroup
	countdown.Add(1)
	go func() {
		defer countdown.Done()
		if duration <= 0 {
			logCapture(start, "controller", "run_started duration=unbounded")
			<-timerCtx.Done()
			return
		}

		deadline := start.Add(duration)
		logCapture(start, "controller", "run_started duration=%s (deadline=%s)", duration, deadline.UTC().Format(time.RFC3339))

		ticker := time.NewTicker(time.Minute)
		timer := time.NewTimer(duration)
		defer ticker.Stop()
		defer timer.Stop()

		remaining := duration
		for {
			select {
			case <-timerCtx.Done():
				return
			case <-ticker.C:
				remaining -= time.Minute
				if remaining <= 0 {
					continue
				}
				minutes := int(remaining / time.Minute)
				if minutes > 0 {
					logCapture(clock(), "controller", "remaining=%dm", minutes)
				}
			case <-timer.C:
				logCapture(clock(), "controller", "duration elapsed; requesting stop")
				if summary.Lifecycle != nil {
					summaryMu.Lock()
					if summary.Lifecycle != nil {
						summary.Lifecycle.TerminationCause = "duration_elapsed"
					}
					summaryMu.Unlock()
				}
				controller.Kill(ErrDurationElapsed)
				return
			}
		}
	}()
	defer func() {
		timerCancel()
		countdown.Wait()
	}()

	eventsEnv := events.DetectEnvironment()
	screenshotsEnv := screenshots.DetectEnvironment()
	videoEnv := video.DetectEnvironment()
	asrEnv := asr.DetectEnvironment(asr.DetectorOptions{WhisperBinary: opts.Config.Capture.ASR.WhisperBinary})
	ocrEnv := ocr.DetectEnvironment(ocr.DetectorOptions{TesseractBinary: opts.Config.Capture.OCR.TesseractBinary})

	baseStatuses := map[string]runmanifest.SubsystemStatus{
		"events": {
			Name:       "events",
			Enabled:    opts.Config.Capture.EventsEnabled,
			Available:  eventsEnv.Available,
			Provider:   eventsEnv.Provider,
			Permission: eventsEnv.Permission,
			Message:    joinMessage(eventsEnv.Message, messageSlice(eventsEnv.Guidance)),
		},
		"screenshots": {
			Name:       "screenshots",
			Enabled:    opts.Config.Capture.ScreenshotsEnabled,
			Available:  screenshotsEnv.Available,
			Provider:   screenshotsEnv.Provider,
			Permission: screenshotsEnv.Permission,
			Message:    joinMessage(screenshotsEnv.Message, messageSlice(screenshotsEnv.Guidance)),
		},
		"video": {
			Name:       "video",
			Enabled:    opts.Config.Capture.VideoEnabled,
			Available:  videoEnv.Available,
			Provider:   videoEnv.Provider,
			Permission: videoEnv.Permission,
			Message:    joinMessage(videoEnv.Message, messageSlice(videoEnv.Guidance)),
		},
		"asr": {
			Name:       "asr",
			Enabled:    opts.Config.Capture.ASREnabled,
			Available:  asrEnv.Available,
			Provider:   asrEnv.Provider,
			Permission: asrEnv.Permission,
			Message:    joinMessage(asrEnv.Message, asrEnv.Guidance),
		},
		"ocr": {
			Name:      "ocr",
			Enabled:   opts.Config.Capture.OCREnabled,
			Available: ocrEnv.Available,
			Provider:  ocrEnv.Provider,
			Message:   joinMessage(ocrEnv.Message, ocrEnv.Guidance),
		},
	}

	statusMu := sync.Mutex{}
	observed := make(map[string]runmanifest.SubsystemStatus)
	recordStatus := func(status runmanifest.SubsystemStatus) {
		statusMu.Lock()
		observed[status.Name] = status
		statusMu.Unlock()
	}

	var screenshotFilesMu sync.Mutex
	var screenshotFiles []string
	screenshotReady := make(chan struct{})
	var screenshotReadyOnce sync.Once
	notifyScreenshotsReady := func(files []string) {
		screenshotFilesMu.Lock()
		screenshotFiles = append([]string(nil), files...)
		screenshotFilesMu.Unlock()
		screenshotReadyOnce.Do(func() { close(screenshotReady) })
	}
	notifyScreenshotsUnavailable := func() {
		screenshotReadyOnce.Do(func() { close(screenshotReady) })
	}
	if !baseStatuses["screenshots"].Enabled || !baseStatuses["screenshots"].Available {
		notifyScreenshotsUnavailable()
	}

	var errOnce sync.Once
	var runErr error

	runners := []subsystemRunner{
		{
			name:   "events",
			status: baseStatuses["events"],
			run: func(runCtx context.Context) (string, func(*Summary), error) {
				opts.Logger.Info("starting event tap capture")
				tap, err := events.NewTap(events.Options{
					FineInterval:   time.Duration(opts.Config.Capture.Events.FineIntervalSeconds) * time.Second,
					CoarseInterval: time.Duration(opts.Config.Capture.Events.CoarseIntervalSeconds) * time.Second,
					Redactor:       redactor,
					Clock:          clock,
					Privacy:        privacy,
				})
				if err != nil {
					return "", nil, err
				}
				res, err := tap.Capture(runCtx, opts.Layout.EventsDir)
				if err != nil {
					return "", nil, err
				}
				logCapture(clock(), "events", "captured %d fine events (%d buckets, %d filtered)", res.EventCount, res.BucketCount, res.FilteredCount)
				opts.Logger.Info("event tap complete", "events", res.EventCount, "buckets", res.BucketCount, "filtered", res.FilteredCount)
				return fmt.Sprintf("%d fine events", res.EventCount), func(s *Summary) { s.Events = &res }, nil
			},
		},
		{
			name:   "screenshots",
			status: baseStatuses["screenshots"],
			run: func(runCtx context.Context) (string, func(*Summary), error) {
				opts.Logger.Info("starting screenshot capture")
				scheduler, err := screenshots.NewScheduler(screenshots.Options{
					Interval:     time.Duration(opts.Config.Capture.Screenshots.IntervalSeconds) * time.Second,
					MaxPerMinute: opts.Config.Capture.Screenshots.MaxPerMinute,
					Clock:        clock,
				})
				if err != nil {
					notifyScreenshotsUnavailable()
					return "", nil, err
				}
				res, err := scheduler.Capture(runCtx, opts.Layout.ScreensDir)
				if err != nil {
					notifyScreenshotsUnavailable()
					return "", nil, err
				}
				notifyScreenshotsReady(res.Files)
				logCapture(clock(), "screenshots", "captured %d screenshots", res.Count)
				opts.Logger.Info("screenshot capture complete", "count", res.Count)
				return fmt.Sprintf("%d captures", res.Count), func(s *Summary) { s.Screenshots = &res }, nil
			},
		},
		{
			name:   "video",
			status: baseStatuses["video"],
			run: func(runCtx context.Context) (string, func(*Summary), error) {
				opts.Logger.Info("starting video capture", "provider", videoEnv.Provider)
				recorder, err := video.NewRecorder(video.Options{
					ChunkSeconds: opts.Config.Capture.Video.ChunkSeconds,
					Format:       opts.Config.Capture.Video.Format,
					Clock:        clock,
				})
				if err != nil {
					return "", nil, err
				}
				res, err := recorder.Record(runCtx, opts.Layout.VideoDir)
				if err != nil {
					opts.Logger.Error("video recorder encountered error", "error", err)
					return "", nil, err
				}
				logCapture(clock(), "video", "captured segment %s", res.File)
				opts.Logger.Info("video capture complete", "file", res.File)
				return fmt.Sprintf("segment -> %s", res.File), func(s *Summary) { s.Video = &res }, nil
			},
		},
		{
			name:   "asr",
			status: baseStatuses["asr"],
			run: func(runCtx context.Context) (string, func(*Summary), error) {
				opts.Logger.Info("starting asr analysis")
				agent, err := asr.NewAgent(asr.Options{
					MeetingKeywords: opts.Config.Capture.ASR.MeetingKeywords,
					WindowTitles:    opts.Config.Capture.ASR.WindowTitles,
					WhisperBinary:   opts.Config.Capture.ASR.WhisperBinary,
					Language:        opts.Config.Capture.ASR.Language,
					Clock:           clock,
					Redactor:        redactor,
				})
				if err != nil {
					return "", nil, err
				}
				res, err := agent.Capture(runCtx, opts.Layout.ASRDir)
				if err != nil {
					return "", nil, err
				}
				logCapture(clock(), "asr", "meeting=%t whisper=%t segments=%d", res.MeetingDetected, res.WhisperAvailable, res.SegmentCount)
				opts.Logger.Info("asr analysis complete", "meeting_detected", res.MeetingDetected, "whisper_available", res.WhisperAvailable, "segments", res.SegmentCount)
				message := "no meeting detected"
				switch {
				case res.TranscriptPath != "":
					message = fmt.Sprintf("transcript -> %s", res.TranscriptPath)
				case res.MeetingDetected:
					message = "meeting detected (no transcript)"
				}
				return message, func(s *Summary) { s.ASR = &res }, nil
			},
		},
		{
			name:   "ocr",
			status: baseStatuses["ocr"],
			run: func(runCtx context.Context) (string, func(*Summary), error) {
				opts.Logger.Info("starting ocr processing")
				worker, err := ocr.NewWorker(ocr.Options{
					Languages:       opts.Config.Capture.OCR.Languages,
					TesseractBinary: opts.Config.Capture.OCR.TesseractBinary,
					Redactor:        redactor,
				})
				if err != nil {
					return "", nil, err
				}
				inputs := make([]string, 0)
				if opts.Config.Capture.ScreenshotsEnabled {
					select {
					case <-screenshotReady:
					case <-runCtx.Done():
						notifyScreenshotsUnavailable()
						return "", nil, runCtx.Err()
					}
				}
				screenshotFilesMu.Lock()
				inputs = append(inputs, screenshotFiles...)
				screenshotFilesMu.Unlock()
				res, err := worker.Process(runCtx, inputs, opts.Layout.OCRDir)
				if err != nil {
					return "", nil, err
				}
				logCapture(clock(), "ocr", "processed=%d skipped=%d", res.ProcessedCount, res.SkippedCount)
				opts.Logger.Info("ocr processing complete", "processed", res.ProcessedCount, "skipped", res.SkippedCount)
				message := fmt.Sprintf("processed=%d skipped=%d", res.ProcessedCount, res.SkippedCount)
				return message, func(s *Summary) { s.OCR = &res }, nil
			},
		},
	}

	var wg sync.WaitGroup
	for _, runner := range runners {
		runner := runner
		wg.Add(1)
		go func() {
			defer wg.Done()

			status := runner.status
			if !status.Enabled {
				status.Message = "disabled in config"
				status.State = runmanifest.SubsystemStateSkipped
				logCapture(clock(), runner.name, "skipped (%s)", status.Message)
				opts.Logger.Info(fmt.Sprintf("%s disabled via config", runner.name))
				if runner.name == "screenshots" {
					notifyScreenshotsUnavailable()
				}
				recordStatus(status)
				return
			}
			if !status.Available {
				if status.Message == "" {
					status.Message = "unavailable"
				}
				status.State = runmanifest.SubsystemStateUnavailable
				logCapture(clock(), runner.name, "unavailable (%s)", status.Message)
				opts.Logger.Warn(fmt.Sprintf("%s unavailable", runner.name), "message", status.Message)
				if runner.name == "screenshots" {
					notifyScreenshotsUnavailable()
				}
				recordStatus(status)
				return
			}

			if err := controller.Wait(runCtx); err != nil {
				switch {
				case errors.Is(err, ErrDurationElapsed):
					status.State = runmanifest.SubsystemStateSkipped
					status.Message = "not run (duration elapsed)"
					summaryMu.Lock()
					if summary.Lifecycle != nil && summary.Lifecycle.TerminationCause == "" {
						summary.Lifecycle.TerminationCause = "duration_elapsed"
					}
					summaryMu.Unlock()
				case errors.Is(err, context.Canceled):
					status.State = runmanifest.SubsystemStateSkipped
					if status.Message == "" {
						status.Message = "not run (controller stopped)"
					}
				default:
					status.State = runmanifest.SubsystemStateErrored
					status.Message = err.Error()
					errOnce.Do(func() { runErr = err })
				}
				if runner.name == "screenshots" {
					notifyScreenshotsUnavailable()
				}
				recordStatus(status)
				return
			}

			message, apply, execErr := runner.run(runCtx)
			if execErr != nil {
				if errors.Is(execErr, context.Canceled) {
					status.State = runmanifest.SubsystemStateSkipped
					if status.Message == "" {
						status.Message = "canceled"
					}
					if runner.name == "screenshots" {
						notifyScreenshotsUnavailable()
					}
					recordStatus(status)
					return
				}
				switch runner.name {
				case "events":
					if errors.Is(execErr, events.ErrAccessibilityPermission) {
						status.State = runmanifest.SubsystemStateUnavailable
						status.Message = joinMessage(status.Message, []string{execErr.Error()})
						recordStatus(status)
						return
					}
				case "screenshots":
					notifyScreenshotsUnavailable()
					if errors.Is(execErr, screenshots.ErrPermissionRequired) {
						status.State = runmanifest.SubsystemStateUnavailable
						status.Message = joinMessage(status.Message, []string{execErr.Error()})
						recordStatus(status)
						return
					}
				case "video":
					if errors.Is(execErr, video.ErrPermissionRequired) {
						status.State = runmanifest.SubsystemStateUnavailable
						status.Message = joinMessage(status.Message, []string{execErr.Error()})
						recordStatus(status)
						return
					}
				}
				status.State = runmanifest.SubsystemStateErrored
				status.Message = execErr.Error()
				recordStatus(status)
				controller.Kill(execErr)
				errOnce.Do(func() { runErr = execErr })
				if runner.name == "screenshots" {
					notifyScreenshotsUnavailable()
				}
				return
			}

			if message != "" {
				status.Message = message
			}
			status.State = runmanifest.SubsystemStateCompleted
			recordStatus(status)

			if apply != nil {
				summaryMu.Lock()
				apply(&summary)
				summaryMu.Unlock()
			}
		}()
	}

	wg.Wait()

	statusOrder := []string{"events", "screenshots", "video", "asr", "ocr"}
	for _, name := range statusOrder {
		base := baseStatuses[name]
		statusMu.Lock()
		observedStatus, ok := observed[name]
		statusMu.Unlock()
		if !ok {
			base.State = runmanifest.SubsystemStateSkipped
			if base.Message == "" {
				base.Message = "not run (controller stopped)"
			}
			observedStatus = base
		}
		summary.Subsystems = append(summary.Subsystems, observedStatus)
	}

	summaryMu.Lock()
	if summary.Lifecycle != nil {
		if summary.Lifecycle.FinishedAt.IsZero() {
			summary.Lifecycle.FinishedAt = clock()
		}
		if summary.Lifecycle.TerminationCause == "" {
			if runErr != nil {
				summary.Lifecycle.TerminationCause = "error"
			} else {
				summary.Lifecycle.TerminationCause = "completed"
			}
		}
	}
	summaryMu.Unlock()

	if runErr != nil {
		err = runErr
	}
	return summary, err
}

func messageSlice(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func joinMessage(base string, guidance []string) string {
	parts := make([]string, 0, 1+len(guidance))
	if strings.TrimSpace(base) != "" {
		parts = append(parts, base)
	}
	for _, g := range guidance {
		trimmed := strings.TrimSpace(g)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "; ")
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
