package version

import (
	"fmt"

	"github.com/couchbase/stellar-gateway/pkg/revision"
)

var (
	Version     string
	BuildNumber string
	Revision    string
)

// This will generate things like 1.0.0 and 1.0.0-beta1 and should be used
// for things like docker images.
func WithRevision() string {
	v := Version

	if Revision != "" {
		v = v + "-" + Revision
	}

	return v
}

// This will generate things like "1.0.0 (build 123)" and should be used for
// binary version strings so we can tell exactly which build (and by extension
// commit) is being used.
func WithBuildNumber() string {
	return fmt.Sprintf("%s (build %s)", WithRevision(), BuildNumber)
}

// WithBuildNumberAndRevision gives full debug information, used primarily for
// CLI commands.
func WithBuildNumberAndRevision() string {
	return fmt.Sprintf("%s (build %s, revision %s)", WithRevision(), BuildNumber, revision.Revision())
}
