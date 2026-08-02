package buildinfo

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestSupplementUsesEmbeddedVCSFallbacks(t *testing.T) {
	info := Info{
		Version:    "dev",
		Commit:     "unknown",
		BuildDate:  "unknown",
		CommitTime: "unknown",
		Modified:   "unknown",
		GoVersion:  "go-test",
		Target:     "linux/amd64",
	}
	embedded := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.time", Value: "2026-08-02T07:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	}

	supplement(&info, embedded)
	if info.Version != "v0.1.0" {
		t.Fatalf("Version = %q", info.Version)
	}
	if info.Commit != "0123456789abcdef" {
		t.Fatalf("Commit = %q", info.Commit)
	}
	if info.BuildDate != "unknown" {
		t.Fatalf("BuildDate = %q", info.BuildDate)
	}
	if info.CommitTime != "2026-08-02T07:00:00Z" {
		t.Fatalf("CommitTime = %q", info.CommitTime)
	}
	if info.Modified != "true" {
		t.Fatalf("Modified = %q", info.Modified)
	}
}

func TestSupplementPreservesInjectedMetadata(t *testing.T) {
	info := Info{
		Version:    "v0.2.0",
		Commit:     "injected",
		CommitTime: "injected-time",
		BuildDate:  "injected-date",
		Modified:   "false",
	}
	want := info
	embedded := &debug.BuildInfo{
		Main: debug.Module{Version: "v9.9.9"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "embedded"},
			{Key: "vcs.time", Value: "embedded-date"},
			{Key: "vcs.modified", Value: "true"},
		},
	}

	supplement(&info, embedded)
	if info.Version != want.Version ||
		info.Commit != want.Commit ||
		info.CommitTime != want.CommitTime ||
		info.BuildDate != want.BuildDate ||
		info.Modified != want.Modified {
		t.Fatalf("supplement() = %#v", info)
	}
}

func TestInfoStringIncludesProvenance(t *testing.T) {
	info := Info{
		Version:         "v0.1.0",
		Commit:          "abc123",
		BuildDate:       "2026-08-02T07:00:00Z",
		CommitTime:      "2026-08-01T18:00:00Z",
		BuiltBy:         "builder",
		BuildHostOS:     "Linux",
		BuildHostKernel: "7.0.0",
		Modified:        "false",
		GoVersion:       "go1.26.5",
		Target:          "linux/amd64",
	}

	text := info.String()
	for _, want := range []string{
		"keebacked v0.1.0",
		"commit: abc123",
		"commit time: 2026-08-01T18:00:00Z",
		"modified: false",
		"built: 2026-08-02T07:00:00Z",
		"built by: builder",
		"build host: Linux 7.0.0",
		"target: linux/amd64",
		"go: go1.26.5",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("String() = %q, missing %q", text, want)
		}
	}
}

func TestNormalizedModifiedRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "unknown", "sometimes"} {
		if got := normalizedModified(value); got != "unknown" {
			t.Fatalf("normalizedModified(%q) = %q", value, got)
		}
	}
}
