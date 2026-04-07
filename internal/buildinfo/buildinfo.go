// Package buildinfo exposes CLI build and version metadata.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

type Info struct {
	Version string
	Commit  string
	Date    string
}

func Read() Info {
	info := Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}

	if build, ok := debug.ReadBuildInfo(); ok {
		info = merge(info, build)
	}

	if info.Version == "" {
		info.Version = "unknown"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.Date == "" {
		info.Date = "unknown"
	}

	return info
}

func String() string {
	info := Read()
	return fmt.Sprintf("møbius\n  version: %s\n  commit: %s\n  built: %s\n", info.Version, info.Commit, info.Date)
}

func merge(info Info, build *debug.BuildInfo) Info {
	if build == nil {
		return info
	}

	if (info.Version == "" || info.Version == "dev") && build.Main.Version != "" && build.Main.Version != "(devel)" {
		info.Version = build.Main.Version
	}

	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.Commit == "" {
				info.Commit = setting.Value
			}
		case "vcs.time":
			if info.Date == "" {
				info.Date = setting.Value
			}
		}
	}

	return info
}
