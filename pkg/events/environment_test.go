package events

import "testing"

func TestDetectEnvironmentSetsFields(t *testing.T) {
	env := DetectEnvironment()
	if env.Provider == "" {
		t.Fatalf("expected provider")
	}
	if env.Permission == "" {
		t.Fatalf("expected permission status")
	}
	if env.Message == "" {
		t.Fatalf("expected message")
	}
}
