package ui

import "runtime/debug"

// Set by release builds; source builds stay visibly marked as development builds.
var version = "dev"

func applicationVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
