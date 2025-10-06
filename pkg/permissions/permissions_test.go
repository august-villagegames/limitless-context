package permissions

import "testing"

type fakeLookup map[string]string

func (f fakeLookup) get(key string) (string, bool) {
	v, ok := f[key]
	return v, ok
}

func TestInterpretPermissionFlag(t *testing.T) {
	cases := map[string]struct {
		value    string
		expected Status
	}{
		"granted":     {"granted", StatusGranted},
		"denied":      {"denied", StatusDenied},
		"prompt":      {"prompt", StatusPromptRequired},
		"unsupported": {"unsupported", StatusUnavailable},
		"unknown":     {"", StatusUnknown},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res := interpretPermissionFlag("test", tc.value)
			if res.Status != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, res.Status)
			}
		})
	}
}

func TestProbeScreenRecordingHonoursEnv(t *testing.T) {
	lookup := fakeLookup{"LIMITLESS_SCREEN_RECORDING": "denied"}
	res := ProbeScreenRecording(lookup.get)
	if res.Status != StatusDenied {
		t.Fatalf("expected denied, got %s", res.Status)
	}
	if res.Guidance == "" {
		t.Fatalf("expected guidance when denied")
	}
}

func TestProbeAccessibilityDefaults(t *testing.T) {
	res := ProbeAccessibility(nil)
	if res.Status == StatusUnknown {
		t.Fatalf("expected platform specific default, got unknown")
	}
}

func TestProbeMicrophoneHonoursEnv(t *testing.T) {
	lookup := fakeLookup{"LIMITLESS_MICROPHONE": "granted"}
	res := ProbeMicrophone(lookup.get)
	if res.Status != StatusGranted {
		t.Fatalf("expected granted, got %s", res.Status)
	}
}
