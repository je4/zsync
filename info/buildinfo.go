package info

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

var VCSRevision string
var VCSTime time.Time
var MainVersion string
var GoVersion string
var BuildInfo string
var Version string
var UserAgent string

func GetUserAgent() string {
	var parts []string
	if MainVersion != "" && MainVersion != "(devel)" {
		parts = append(parts, MainVersion)
	}
	if VCSRevision != "" {
		parts = append(parts, VCSRevision)
	}
	if !VCSTime.IsZero() {
		parts = append(parts, VCSTime.Format(time.RFC3339))
	}
	if GoVersion != "" {
		parts = append(parts, GoVersion)
	}
	if len(parts) == 0 {
		return "zsync"
	}
	return fmt.Sprintf("zsync/%s", strings.Join(parts, " "))
}

func init() {
	UserAgent = GetUserAgent()
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	BuildInfo = info.String()
	MainVersion = info.Main.Version
	GoVersion = info.GoVersion
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			VCSRevision = setting.Value
		case "vcs.time":
			VCSTime, _ = time.Parse(time.RFC3339, setting.Value)
		}
	}
	Version = fmt.Sprintf("%s %s (%s) %s", MainVersion, VCSRevision, VCSTime.Format(time.RFC3339), GoVersion)
	UserAgent = GetUserAgent()
}
