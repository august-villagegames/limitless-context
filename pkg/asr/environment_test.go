package asr

import (
	"errors"
	"testing"
)

type fakeLookPath struct {
	err error
}

func (f fakeLookPath) lookup(string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "/usr/local/bin/whisper", nil
}

func TestDetectEnvironmentReportsWhisperAvailability(t *testing.T) {
	env := DetectEnvironment(DetectorOptions{WhisperBinary: "whisper", LookPath: fakeLookPath{}.lookup})
	if !env.WhisperAvailable {
		t.Fatalf("expected whisper to be available")
	}
	if !env.Available {
		t.Fatalf("expected environment to be available when dependencies satisfied")
	}
}

func TestDetectEnvironmentWhenWhisperMissing(t *testing.T) {
	env := DetectEnvironment(DetectorOptions{WhisperBinary: "whisper", LookPath: fakeLookPath{err: errors.New("missing")}.lookup})
	if env.WhisperAvailable {
		t.Fatalf("expected whisper to be unavailable")
	}
	if len(env.Guidance) == 0 {
		t.Fatalf("expected guidance when whisper missing")
	}
	if env.Available { // still runs in stub mode
		return
	}
}
