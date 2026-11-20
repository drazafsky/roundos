// Package release publishes RoundOS's release identity without depending on
// /etc, DRM devices, or network availability.
package release

const Version = "0.5.0"

// BuildID may be set at link time from an audited release build.
var BuildID = "source"
