package log

import "testing"

func TestSetLevelAndEmit(t *testing.T) {
	levels := []string{"error", "info", "debug", "ERROR", "Info", "unknown", ""}
	for _, level := range levels {
		SetLevel(level)
		// Smoke: must not panic at any configured level.
		Debug("debug %s", level)
		Info("info %s", level)
		Error("error %s", level)
	}
}

func TestSetLevelValues(t *testing.T) {
	tests := []struct {
		in   string
		want Level
	}{
		{"error", LevelError},
		{"info", LevelInfo},
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"nope", LevelError},
		{"", LevelError},
	}
	for _, tc := range tests {
		SetLevel(tc.in)
		if currentLevel != tc.want {
			t.Fatalf("SetLevel(%q): got %v want %v", tc.in, currentLevel, tc.want)
		}
	}
}
