// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_buildVersion_reads_the_main_module_version(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{name: "no build info", info: nil, want: ""},
		{name: "a build with no module version", info: &debug.BuildInfo{}, want: ""},
		{name: "a tagged build", info: &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, want: "v1.2.3"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, buildVersion(c.info))
		})
	}
}
