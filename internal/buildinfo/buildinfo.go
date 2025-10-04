package buildinfo

import "runtime/debug"

var version = "dev"

// SetVersion allows build scripts to override the CLI version information.
func SetVersion(v string) {
	if v == "" {
		return
	}
	version = v
}

// Version returns the semantic version or commit hash associated with the build.
func Version() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}
