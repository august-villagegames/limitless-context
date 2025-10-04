package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sort"

	"github.com/offlinefirst/limitless-context/internal/buildinfo"
	"github.com/offlinefirst/limitless-context/pkg/config"
	"github.com/offlinefirst/limitless-context/pkg/logging"
)

type command struct {
	name        string
	description string
	configure   func(fs *flag.FlagSet)
	run         func(fs *flag.FlagSet, args []string, ctx *AppContext, stdout io.Writer, stderr io.Writer) error
	skipInit    bool
}

// AppContext exposes lazily initialised configuration and logging facilities.
type AppContext struct {
	Config config.Config
	Logger *slog.Logger
}

type RootCommand struct {
	commands   map[string]command
	stdout     io.Writer
	stderr     io.Writer
	appCtx     *AppContext
	configPath string
	logLevel   string
	logFormat  string
}

// NewRootCommand constructs the CLI dispatcher with roadmap-aligned subcommands and flag handling.
func NewRootCommand() *RootCommand {
	rc := &RootCommand{
		commands: make(map[string]command),
		stdout:   os.Stdout,
		stderr:   os.Stderr,
	}

	rc.register(newBootstrapCommand())
	rc.register(newRunCommand())
	rc.register(newBundleCommand())
	rc.register(newProcessCommand())
	rc.register(newReportCommand())
	rc.register(newCleanCommand())
	rc.register(newDoctorCommand())
	rc.register(newVersionCommand())

	return rc
}

func (rc *RootCommand) register(cmd command) {
	rc.commands[cmd.name] = cmd
}

// Execute evaluates the supplied arguments, parses global flags, and dispatches to a subcommand.
func (rc *RootCommand) Execute(args []string) error {
	rootFlags := flag.NewFlagSet("tester", flag.ContinueOnError)
	rootFlags.SetOutput(rc.stderr)
	rootFlags.Usage = func() { rc.printHelp() }

	rootFlags.StringVar(&rc.configPath, "config", "", "Path to config file (default: ./config.yaml if present)")
	rootFlags.StringVar(&rc.logLevel, "log-level", "", "Override log level (debug, info, warn, error)")
	rootFlags.StringVar(&rc.logFormat, "log-format", "", "Override log output format (json, console)")

	if err := rootFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			rc.printHelp()
			return nil
		}
		return err
	}

	remaining := rootFlags.Args()
	if len(remaining) == 0 {
		rc.printHelp()
		return nil
	}

	subcommand, ok := rc.commands[remaining[0]]
	if !ok {
		fmt.Fprintf(rc.stderr, "Unknown command %q\n\n", remaining[0])
		rc.printHelp()
		return fmt.Errorf("unknown command")
	}

	fs := flag.NewFlagSet(subcommand.name, flag.ContinueOnError)
	fs.SetOutput(rc.stderr)
	fs.Usage = func() {
		fmt.Fprintf(rc.stdout, "Usage: tester %s [flags]\n", subcommand.name)
		if subcommand.description != "" {
			fmt.Fprintln(rc.stdout, subcommand.description)
		}
		fs.PrintDefaults()
	}

	if subcommand.configure != nil {
		subcommand.configure(fs)
	}

	if err := fs.Parse(remaining[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	var ctx *AppContext
	var err error
	if !subcommand.skipInit {
		if ctx, err = rc.ensureAppContext(); err != nil {
			return err
		}
	}

	return subcommand.run(fs, fs.Args(), ctx, rc.stdout, rc.stderr)
}

func (rc *RootCommand) ensureAppContext() (*AppContext, error) {
	if rc.appCtx != nil {
		return rc.appCtx, nil
	}

	cfg, err := config.Load(rc.configPath)
	if err != nil {
		return nil, err
	}

	if rc.logLevel != "" {
		lvl, err := config.NormalizeLogLevel(rc.logLevel)
		if err != nil {
			return nil, err
		}
		cfg.Logging.Level = lvl
	}
	if rc.logFormat != "" {
		format, err := config.NormalizeFormat(rc.logFormat)
		if err != nil {
			return nil, err
		}
		cfg.Logging.Format = format
	}

	logger, err := logging.New(logging.Options{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		Output: rc.stderr,
	})
	if err != nil {
		return nil, err
	}

	logger.Info("configuration loaded", "source", cfg.Source, "runs_dir", cfg.Paths.RunsDir, "cache_dir", cfg.Paths.CacheDir)

	rc.appCtx = &AppContext{Config: cfg, Logger: logger}
	return rc.appCtx, nil
}

func (rc *RootCommand) printHelp() {
	fmt.Fprintf(rc.stdout, "tester - offline capture CLI\nVersion: %s\n\n", versionString())
	fmt.Fprintln(rc.stdout, "Usage: tester [global flags] <command> [command flags]")
	fmt.Fprintln(rc.stdout, "Global flags:")
	fmt.Fprintln(rc.stdout, "  --config string      Path to config file (default: ./config.yaml if present)")
	fmt.Fprintln(rc.stdout, "  --log-level string   Override log level (debug, info, warn, error)")
	fmt.Fprintln(rc.stdout, "  --log-format string  Override log output format (json, console)")
	fmt.Fprintln(rc.stdout, "")
	fmt.Fprintln(rc.stdout, "Available commands:")

	names := make([]string, 0, len(rc.commands))
	for name := range rc.commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(rc.stdout, "  %-10s %s\n", name, rc.commands[name].description)
	}
}

func versionString() string {
	return fmt.Sprintf("%s (go%s/%s)", buildinfo.Version(), runtimeVersion(), runtimeGOOS())
}

// runtimeVersion is extracted for testability.
var runtimeVersion = func() string { return runtime.Version() }

// runtimeGOOS is extracted for testability.
var runtimeGOOS = func() string { return runtime.GOOS }
