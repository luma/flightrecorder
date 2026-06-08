package env_test

import (
	"fmt"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/luma/flightrecorder/env"
)

var _ = Describe("env/GetInfo()", func() {
	It("returns info about the build environment", func() {
		info := env.GetInfo()

		Expect(info).To(Equal(env.Info{
			GoVersion: runtime.Version(),
			Version:   env.Version,
			Build:     env.Build,
			Branch:    env.Branch,
			BuildTime: env.BuildTimeUTC,
			GoTag:     env.GoTag,
			Platform:  fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH),
		}))
	})
})
