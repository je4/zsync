package info

import (
	"testing"
	"time"
)

func TestGetUserAgent(t *testing.T) {
	origMainVersion := MainVersion
	origVCSRevision := VCSRevision
	origVCSTime := VCSTime
	origGoVersion := GoVersion
	origUserAgent := UserAgent
	defer func() {
		MainVersion = origMainVersion
		VCSRevision = origVCSRevision
		VCSTime = origVCSTime
		GoVersion = origGoVersion
		UserAgent = origUserAgent
	}()

	testTime, err := time.Parse(time.RFC3339, "2026-03-31T12:00:00Z")
	if err != nil {
		t.Fatalf("failed to parse test time: %v", err)
	}

	tests := []struct {
		name        string
		mainVersion string
		vcsRevision string
		vcsTime     time.Time
		goVersion   string
		expected    string
	}{
		{
			name:        "all empty",
			mainVersion: "",
			vcsRevision: "",
			vcsTime:     time.Time{},
			goVersion:   "",
			expected:    "zsync",
		},
		{
			name:        "devel version with zero time",
			mainVersion: "(devel)",
			vcsRevision: "",
			vcsTime:     time.Time{},
			goVersion:   "",
			expected:    "zsync",
		},
		{
			name:        "main version only",
			mainVersion: "v2.1.0",
			vcsRevision: "",
			vcsTime:     time.Time{},
			goVersion:   "",
			expected:    "zsync/v2.1.0",
		},
		{
			name:        "revision and go version only",
			mainVersion: "",
			vcsRevision: "a1b2c3d",
			vcsTime:     time.Time{},
			goVersion:   "go1.26",
			expected:    "zsync/a1b2c3d go1.26",
		},
		{
			name:        "all fields populated",
			mainVersion: "v2.1.0",
			vcsRevision: "a1b2c3d",
			vcsTime:     testTime,
			goVersion:   "go1.26",
			expected:    "zsync/v2.1.0 a1b2c3d 2026-03-31T12:00:00Z go1.26",
		},
		{
			name:        "devel with revision and time and go version",
			mainVersion: "(devel)",
			vcsRevision: "abc456",
			vcsTime:     testTime,
			goVersion:   "go1.26",
			expected:    "zsync/abc456 2026-03-31T12:00:00Z go1.26",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			MainVersion = tc.mainVersion
			VCSRevision = tc.vcsRevision
			VCSTime = tc.vcsTime
			GoVersion = tc.goVersion

			ua := GetUserAgent()
			if ua != tc.expected {
				t.Errorf("GetUserAgent() = %q, expected %q", ua, tc.expected)
			}
		})
	}
}
