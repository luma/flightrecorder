package env

import (
	"fmt"
	"runtime"
)

// Info describes the build context info for the binary.
//
// It encapsulates a bunch of information that's included at build time
// by the Go linker. See the vars below for more information.
type Info struct {
	Version   string `json:"version"`
	Build     string `json:"build"`
	Branch    string `json:"branch"`
	BuildTime string `json:"buildTime"`
	Platform  string `json:"platform"`
	GoVersion string `json:"goVersion"`
	GoTag     string `json:"goTag"`
}

// These will be filled in using the linker -X flag.
var (
	// Version as an arbitrary string.
	Version string

	// Build is the Git sha from when we are building.
	Build string

	// Branch is the Git branch that we are building from.
	Branch string

	// BuildTimeUTC is the build time in UTC (year/month/day hour:min:sec).
	BuildTimeUTC string

	// Go Tag is the Go build tags. See the following references for more info.
	//
	// * https://golang.org/pkg/go/build/#hdr-Build_Constraints
	// * https://www.digitalocean.com/community/tutorials/customizing-go-binaries-with-build-tags
	// * https://dave.cheney.net/2013/10/12/how-to-use-conditional-compilation-with-the-go-build-tool
	//
	GoTag string

	platform = fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH)
)

// GetInfo returns an Info struct populated with the build information.
func GetInfo() Info {
	return Info{
		GoVersion: runtime.Version(),
		Version:   Version,
		Build:     Build,
		Branch:    Branch,
		BuildTime: BuildTimeUTC,
		GoTag:     GoTag,
		Platform:  platform,
	}
}
