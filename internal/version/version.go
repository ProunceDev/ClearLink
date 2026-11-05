package version

import (
	"os"
	"runtime/debug"
	"strings"
)

// Version can be set at link time with -ldflags "-X clearlink/internal/version.Version=<hash>".
var Version = "dev"

const shortLength = 8

// Get returns the short commit hash this binary is running.
func Get() string {
	if value := strings.TrimSpace(os.Getenv("CLEARLINK_VERSION")); value != "" {
		return shorten(value)
	}
	if Version != "" && Version != "dev" {
		return shorten(Version)
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		revision, dirty := "", false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
		if revision != "" {
			if dirty {
				return shorten(revision) + "-dirty"
			}
			return shorten(revision)
		}
	}
	return "dev"
}

func shorten(value string) string {
	if len(value) > shortLength {
		return value[:shortLength]
	}
	return value
}
