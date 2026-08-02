// Package buildinfo reports keebacked release and build provenance.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

var (
	version         = "dev"
	commit          = "unknown"
	commitTime      = "unknown"
	buildDate       = "unknown"
	builtBy         = "unknown"
	buildHostOS     = "unknown"
	buildHostKernel = "unknown"
	modified        = "unknown"
)

// Info describes the running keebacked binary.
type Info struct {
	Version         string
	Commit          string
	BuildDate       string
	CommitTime      string
	BuiltBy         string
	BuildHostOS     string
	BuildHostKernel string
	Modified        string
	GoVersion       string
	Target          string
}

// Current returns build provenance from linker-injected values, supplemented by
// Go's embedded build information when available.
func Current() Info {
	info := Info{
		Version:         normalized(version, "dev"),
		Commit:          normalized(commit, "unknown"),
		BuildDate:       normalized(buildDate, "unknown"),
		CommitTime:      normalized(commitTime, "unknown"),
		BuiltBy:         normalized(builtBy, "unknown"),
		BuildHostOS:     normalized(buildHostOS, "unknown"),
		BuildHostKernel: normalized(buildHostKernel, "unknown"),
		Modified:        normalizedModified(modified),
		GoVersion:       runtime.Version(),
		Target:          runtime.GOOS + "/" + runtime.GOARCH,
	}

	if embedded, ok := debug.ReadBuildInfo(); ok {
		supplement(&info, embedded)
	}
	return info
}

// String returns a human-readable multi-line version report.
func (i *Info) String() string {
	return fmt.Sprintf(
		"keebacked %s\ncommit: %s\ncommit time: %s\nmodified: %s\nbuilt: %s\nbuilt by: %s\nbuild host: %s %s\ntarget: %s\ngo: %s\n",
		i.Version,
		i.Commit,
		i.CommitTime,
		i.Modified,
		i.BuildDate,
		i.BuiltBy,
		i.BuildHostOS,
		i.BuildHostKernel,
		i.Target,
		i.GoVersion,
	)
}

func supplement(info *Info, embedded *debug.BuildInfo) {
	if info.GoVersion == "" && embedded.GoVersion != "" {
		info.GoVersion = embedded.GoVersion
	}
	if (info.Version == "dev" || info.Version == "unknown") && embedded.Main.Version != "" && embedded.Main.Version != "(devel)" {
		info.Version = embedded.Main.Version
	}

	settings := make(map[string]string, len(embedded.Settings))
	for _, setting := range embedded.Settings {
		settings[setting.Key] = setting.Value
	}
	if info.Commit == "unknown" {
		info.Commit = normalized(settings["vcs.revision"], "unknown")
	}
	if info.CommitTime == "unknown" {
		info.CommitTime = normalized(settings["vcs.time"], "unknown")
	}
	if info.Modified == "unknown" {
		info.Modified = normalizedModified(settings["vcs.modified"])
	}
}

func normalized(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizedModified(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "unknown" {
		return "unknown"
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return "unknown"
	}
	return strconv.FormatBool(parsed)
}
