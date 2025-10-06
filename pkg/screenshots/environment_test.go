package screenshots

import "testing"

func TestDetectEnvironmentPopulatesFields(t *testing.T) {
	env := DetectEnvironment()
	if env.Provider == "" {
		t.Fatalf("expected provider name")
	}
	if env.Permission == "" {
		t.Fatalf("expected permission string")
	}
	if env.Message == "" {
		t.Fatalf("expected message")
	}
}
