// Package buildinfo holds link-time version metadata shared by the version
// command and the Spur API HTTP User-Agent.
package buildinfo

// Version is the release or git tag (e.g. v0.2.2). Overridden at link time via
// -ldflags; defaults to "dev" for a local go build.
var Version = "dev"

// BuildStamp is an opaque build identifier (release builds inject the UTC
// date). Overridden at link time; defaults to "dev".
var BuildStamp = "dev"

// String returns the human-readable build string reported by `spur version`.
func String() string {
	return Version + " (" + BuildStamp + ")"
}
