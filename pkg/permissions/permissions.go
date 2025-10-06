package permissions

import (
	"os"
	"runtime"
	"strings"
)

// Status enumerates coarse permission results for macOS-style prompts.
type Status string

const (
	// StatusUnknown indicates no explicit signal about permission state.
	StatusUnknown Status = "unknown"
	// StatusGranted signals that permission was previously granted.
	StatusGranted Status = "granted"
	// StatusDenied indicates the user has explicitly denied access.
	StatusDenied Status = "denied"
	// StatusPromptRequired means the platform will prompt at runtime.
	StatusPromptRequired Status = "prompt"
	// StatusUnavailable reports that the capability is not supported.
	StatusUnavailable Status = "unavailable"
)

// ProbeResult represents the coarse state for a permission surface.
type ProbeResult struct {
	Status   Status
	Message  string
	Guidance string
}

// LookupEnvFunc exposes environment probing for testability.
type LookupEnvFunc func(string) (string, bool)

// DefaultLookupEnv is the standard environment resolver.
func DefaultLookupEnv(key string) (string, bool) {
	return lookupEnv(key)
}

// lookupEnv is declared for swapping in tests.
var lookupEnv = func(key string) (string, bool) {
	return os.LookupEnv(key)
}

// ProbeScreenRecording inspects the execution environment for screen recording permissions.
func ProbeScreenRecording(lookup LookupEnvFunc) ProbeResult {
	if lookup == nil {
		lookup = lookupEnv
	}
	if value, ok := lookup("LIMITLESS_SCREEN_RECORDING"); ok {
		return interpretPermissionFlag("screen recording", value)
	}
	if runtime.GOOS == "darwin" {
		return ProbeResult{Status: StatusPromptRequired, Message: "awaiting macOS screen recording authorisation"}
	}
	return ProbeResult{Status: StatusUnavailable, Message: "screen recording unsupported on this platform"}
}

// ProbeAccessibility inspects environment flags for accessibility trust.
func ProbeAccessibility(lookup LookupEnvFunc) ProbeResult {
	if lookup == nil {
		lookup = lookupEnv
	}
	if value, ok := lookup("LIMITLESS_ACCESSIBILITY"); ok {
		return interpretPermissionFlag("accessibility", value)
	}
	if runtime.GOOS == "darwin" {
		return ProbeResult{Status: StatusPromptRequired, Message: "accessibility trust required"}
	}
	return ProbeResult{Status: StatusUnavailable, Message: "accessibility prompts unavailable"}
}

// ProbeMicrophone reports coarse microphone capture permissions.
func ProbeMicrophone(lookup LookupEnvFunc) ProbeResult {
	if lookup == nil {
		lookup = lookupEnv
	}
	if value, ok := lookup("LIMITLESS_MICROPHONE"); ok {
		return interpretPermissionFlag("microphone", value)
	}
	if runtime.GOOS == "darwin" {
		return ProbeResult{Status: StatusPromptRequired, Message: "microphone access will prompt at runtime"}
	}
	return ProbeResult{Status: StatusUnavailable, Message: "microphone capture unsupported"}
}

func interpretPermissionFlag(name, value string) ProbeResult {
	normalised := strings.ToLower(strings.TrimSpace(value))
	switch normalised {
	case "granted", "allow", "allowed", "yes", "true":
		return ProbeResult{Status: StatusGranted, Message: name + " permission pre-authorised via env override"}
	case "denied", "no", "false", "blocked":
		return ProbeResult{Status: StatusDenied, Message: name + " permission denied via env override", Guidance: "use 'tccutil reset' or update LIMITLESS_* env to re-test"}
	case "prompt", "ask":
		return ProbeResult{Status: StatusPromptRequired, Message: name + " permission will prompt at runtime"}
	case "unavailable", "unsupported":
		return ProbeResult{Status: StatusUnavailable, Message: name + " permission unavailable on this platform"}
	default:
		return ProbeResult{Status: StatusUnknown, Message: name + " permission state unknown"}
	}
}

// StatusString returns the string representation for manifest integration.
func (p ProbeResult) StatusString() string {
	if p.Status == "" {
		return string(StatusUnknown)
	}
	return string(p.Status)
}
