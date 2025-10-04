package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const DefaultFileName = "config.yaml"

// Config captures the user-adjustable knobs for the capture workflows.
type Config struct {
        Paths   PathsConfig
        Capture CaptureConfig
        Logging LoggingConfig

        // Source indicates where the configuration originated (defaults or a file path).
        Source string
}

// PathsConfig controls filesystem locations used by the CLI.
type PathsConfig struct {
	RunsDir  string
	CacheDir string
}

// CaptureConfig toggles capture subsystems.
type CaptureConfig struct {
        VideoEnabled       bool
        ScreenshotsEnabled bool
        EventsEnabled      bool

        Video       VideoConfig
        Screenshots ScreenshotConfig
        Events      EventsConfig
}

// VideoConfig defines options for the video recorder stub.
type VideoConfig struct {
        ChunkSeconds int
        Format       string
}

// ScreenshotConfig controls screenshot cadence and throttling.
type ScreenshotConfig struct {
        IntervalSeconds int
        MaxPerMinute    int
}

// EventsConfig configures the event tap capture pipeline.
type EventsConfig struct {
        FineIntervalSeconds   int
        CoarseIntervalSeconds int
        RedactEmails          bool
        RedactPatterns        []string
}

// LoggingConfig defines log verbosity and formatting.
type LoggingConfig struct {
	Level  string
	Format string
}

// Default returns the baseline configuration used when no overrides are supplied.
func Default() Config {
        return Config{
                Paths: PathsConfig{
                        RunsDir:  "runs",
                        CacheDir: "cache",
                },
                Capture: CaptureConfig{
                        VideoEnabled:       true,
                        ScreenshotsEnabled: true,
                        EventsEnabled:      true,
                        Video: VideoConfig{
                                ChunkSeconds: 300,
                                Format:       "webm",
                        },
                        Screenshots: ScreenshotConfig{
                                IntervalSeconds: 60,
                                MaxPerMinute:    3,
                        },
                        Events: EventsConfig{
                                FineIntervalSeconds:   10,
                                CoarseIntervalSeconds: 60,
                                RedactEmails:          true,
                                RedactPatterns:        nil,
                        },
                },
                Logging: LoggingConfig{
                        Level:  "info",
                        Format: "json",
                },
		Source: "<defaults>",
	}
}

// Load reads configuration from disk if present, otherwise returning defaults.
// When path is empty, the loader attempts to read ./config.yaml but tolerates a missing file.
func Load(path string) (Config, error) {
	cfg := Default()

	candidate := strings.TrimSpace(path)
	explicit := candidate != ""
	if !explicit {
		candidate = DefaultFileName
	}

	file, err := os.Open(candidate)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if explicit {
				return cfg, fmt.Errorf("config file %q not found", candidate)
			}
			return cfg, nil
		}
		return cfg, fmt.Errorf("open config file %q: %w", candidate, err)
	}
	defer file.Close()

	if err := decodeYAML(file, &cfg); err != nil {
		return cfg, err
	}
	cfg.Source = candidate
	cfg.normalize()

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// Validate ensures essential configuration values are present and sensible.
func (c Config) Validate() error {
        if strings.TrimSpace(c.Paths.RunsDir) == "" {
                return errors.New("paths.runs_dir must not be empty")
        }
        if strings.TrimSpace(c.Paths.CacheDir) == "" {
                return errors.New("paths.cache_dir must not be empty")
        }

        if _, err := NormalizeLogLevel(c.Logging.Level); err != nil {
                return err
        }
        if _, err := NormalizeFormat(c.Logging.Format); err != nil {
                return err
        }

        if c.Capture.Video.ChunkSeconds <= 0 {
                return errors.New("capture.video.chunk_seconds must be positive")
        }
        if strings.TrimSpace(c.Capture.Video.Format) == "" {
                return errors.New("capture.video.format must not be empty")
        }
        if c.Capture.Screenshots.IntervalSeconds <= 0 {
                return errors.New("capture.screenshots.interval_seconds must be positive")
        }
        if c.Capture.Screenshots.MaxPerMinute <= 0 {
                return errors.New("capture.screenshots.max_per_minute must be positive")
        }
        if c.Capture.Events.FineIntervalSeconds <= 0 {
                return errors.New("capture.events.fine_interval_seconds must be positive")
        }
        if c.Capture.Events.CoarseIntervalSeconds <= 0 {
                return errors.New("capture.events.coarse_interval_seconds must be positive")
        }

        return nil
}

// decodeYAML ingests a small subset of YAML to avoid external dependencies.
type yamlFrame struct {
	indent int
	key    string
}

func decodeYAML(r io.Reader, cfg *Config) error {
	scanner := bufio.NewScanner(r)
	var stack []yamlFrame

	lineNo := 0
	for scanner.Scan() {
		raw := scanner.Text()
		lineNo++

		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := countIndent(raw)
		if indent%2 != 0 {
			return fmt.Errorf("line %d: indentation must be multiples of two spaces", lineNo)
		}

		for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}

		key, value, hasValue := splitKeyValue(trimmed)
		if !hasValue {
			stack = append(stack, yamlFrame{indent: indent, key: key})
			continue
		}

		if err := applyValue(cfg, stack, key, value); err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	return nil
}

func countIndent(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func splitKeyValue(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	key := strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		return key, "", false
	}
	value := strings.TrimSpace(parts[1])
	if value == "" {
		return key, "", false
	}
	return key, value, true
}

func applyValue(cfg *Config, stack []yamlFrame, key, rawValue string) error {
	value := sanitizeValue(rawValue)
	path := make([]string, 0, len(stack)+1)
	for _, fr := range stack {
		path = append(path, fr.key)
	}
	path = append(path, key)

	switch strings.Join(path, ".") {
	case "paths.runs_dir":
		cfg.Paths.RunsDir = value
	case "paths.cache_dir":
		cfg.Paths.CacheDir = value
	case "capture.video_enabled":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("capture.video_enabled: %w", err)
		}
		cfg.Capture.VideoEnabled = b
	case "capture.screenshots_enabled":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("capture.screenshots_enabled: %w", err)
		}
		cfg.Capture.ScreenshotsEnabled = b
	case "capture.events_enabled":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("capture.events_enabled: %w", err)
		}
		cfg.Capture.EventsEnabled = b
	case "logging.level":
		cfg.Logging.Level = strings.ToLower(value)
	case "logging.format":
		cfg.Logging.Format = strings.ToLower(value)
        case "capture.video.chunk_seconds":
                seconds, err := parseInt(value)
                if err != nil {
                        return fmt.Errorf("capture.video.chunk_seconds: %w", err)
                }
                cfg.Capture.Video.ChunkSeconds = seconds
        case "capture.video.format":
                cfg.Capture.Video.Format = strings.ToLower(value)
        case "capture.screenshots.interval_seconds":
                seconds, err := parseInt(value)
                if err != nil {
                        return fmt.Errorf("capture.screenshots.interval_seconds: %w", err)
                }
                cfg.Capture.Screenshots.IntervalSeconds = seconds
        case "capture.screenshots.max_per_minute":
                limit, err := parseInt(value)
                if err != nil {
                        return fmt.Errorf("capture.screenshots.max_per_minute: %w", err)
                }
                cfg.Capture.Screenshots.MaxPerMinute = limit
        case "capture.events.fine_interval_seconds":
                seconds, err := parseInt(value)
                if err != nil {
                        return fmt.Errorf("capture.events.fine_interval_seconds: %w", err)
                }
                cfg.Capture.Events.FineIntervalSeconds = seconds
        case "capture.events.coarse_interval_seconds":
                seconds, err := parseInt(value)
                if err != nil {
                        return fmt.Errorf("capture.events.coarse_interval_seconds: %w", err)
                }
                cfg.Capture.Events.CoarseIntervalSeconds = seconds
        case "capture.events.redact_emails":
                b, err := parseBool(value)
                if err != nil {
                        return fmt.Errorf("capture.events.redact_emails: %w", err)
                }
                cfg.Capture.Events.RedactEmails = b
        case "capture.events.redact_patterns":
                cfg.Capture.Events.RedactPatterns = parseList(value)
        default:
                return fmt.Errorf("unknown key %q", strings.Join(path, "."))
        }

        return nil
}

func sanitizeValue(raw string) string {
	value := raw
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = value[:idx]
	}
	if idx := strings.Index(value, "\t#"); idx >= 0 {
		value = value[:idx]
	}
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "'\"")
	return value
}

func parseBool(value string) (bool, error) {
        switch strings.ToLower(value) {
        case "true", "yes", "on":
                return true, nil
        case "false", "no", "off":
                return false, nil
        default:
                return false, fmt.Errorf("invalid boolean value %q", value)
        }
}

func parseInt(value string) (int, error) {
        var i int
        _, err := fmt.Sscanf(value, "%d", &i)
        if err != nil {
                return 0, fmt.Errorf("invalid integer value %q", value)
        }
        return i, nil
}

func parseList(value string) []string {
        if strings.TrimSpace(value) == "" {
                return nil
        }
        parts := strings.Split(value, ",")
        out := make([]string, 0, len(parts))
        for _, part := range parts {
                trimmed := strings.TrimSpace(part)
                if trimmed != "" {
                        out = append(out, trimmed)
                }
        }
        if len(out) == 0 {
                return nil
        }
        return out
}

func (c *Config) normalize() {
        c.Paths.RunsDir = filepath.Clean(strings.TrimSpace(c.Paths.RunsDir))
        c.Paths.CacheDir = filepath.Clean(strings.TrimSpace(c.Paths.CacheDir))

        defaults := Default()

	if c.Paths.RunsDir == "." || c.Paths.RunsDir == "" {
		c.Paths.RunsDir = defaults.Paths.RunsDir
	}
	if c.Paths.CacheDir == "." || c.Paths.CacheDir == "" {
		c.Paths.CacheDir = defaults.Paths.CacheDir
	}
        if strings.TrimSpace(c.Logging.Level) == "" {
                c.Logging.Level = defaults.Logging.Level
        }
        if strings.TrimSpace(c.Logging.Format) == "" {
                c.Logging.Format = defaults.Logging.Format
        }

        if c.Capture.Video.ChunkSeconds <= 0 {
                c.Capture.Video.ChunkSeconds = defaults.Capture.Video.ChunkSeconds
        }
        if strings.TrimSpace(c.Capture.Video.Format) == "" {
                c.Capture.Video.Format = defaults.Capture.Video.Format
        }
        if c.Capture.Screenshots.IntervalSeconds <= 0 {
                c.Capture.Screenshots.IntervalSeconds = defaults.Capture.Screenshots.IntervalSeconds
        }
        if c.Capture.Screenshots.MaxPerMinute <= 0 {
                c.Capture.Screenshots.MaxPerMinute = defaults.Capture.Screenshots.MaxPerMinute
        }
        if c.Capture.Events.FineIntervalSeconds <= 0 {
                c.Capture.Events.FineIntervalSeconds = defaults.Capture.Events.FineIntervalSeconds
        }
        if c.Capture.Events.CoarseIntervalSeconds <= 0 {
                c.Capture.Events.CoarseIntervalSeconds = defaults.Capture.Events.CoarseIntervalSeconds
        }
}

// NormalizeLogLevel validates and lowercases known logging levels.
func NormalizeLogLevel(level string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return "info", nil
	case "debug":
		return "debug", nil
	case "warn", "warning":
		return "warn", nil
	case "error":
		return "error", nil
	default:
		return "", fmt.Errorf("unsupported log level %q", level)
	}
}

// NormalizeFormat validates and canonicalizes logging format identifiers.
func NormalizeFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return "json", nil
	case "console", "text":
		return "console", nil
	default:
		return "", fmt.Errorf("unsupported log format %q", format)
	}
}
