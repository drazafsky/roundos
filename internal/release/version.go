// Package release publishes RoundOS's release identity without depending on
// /etc, DRM devices, or network availability.
package release

// Version defaults to a development build and is set by scripts/build.sh.
var Version = "0.5.1-dev"

// BuildID may be set at link time from an audited release build.
var BuildID = "source"
