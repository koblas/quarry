package version_test

import (
	"runtime"
	"runtime/debug"
	"slices"
	"testing"

	"github.com/koblas/quarry/internal/version"
	"github.com/stretchr/testify/assert"
)

const (
	revisionKey   = "vcs.revision"
	modifiedKey   = "vcs.modified"
	develVersion  = "(devel)"
	pseudoVersion = "v0.0.0-20261121151146-260d0d29f940"
	revision      = "260d0d29f940c1a7e0b5d3c8a1f2e4b6d8a0c2e4"
	shortRevision = "260d0d29f940"
	otherRevision = "9f8e7d6c5b4a39281706f5e4d3c2b1a098765432"
)

// buildOf is build info with the given main module version and settings,
// given as key, value pairs; a repeated key is a duplicate setting.
func buildOf(mainVersion string, keyValues ...string) *debug.BuildInfo {
	info := &debug.BuildInfo{Main: debug.Module{Version: mainVersion}}
	for pair := range slices.Chunk(keyValues, 2) {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: pair[0], Value: pair[1]})
	}
	return info
}

func resolved(versionText string, commit *string, modified *bool) version.Info {
	return version.Info{Version: versionText, Commit: commit, Modified: modified, Go: runtime.Version()}
}

func Test_Resolve(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		want version.Info
	}{
		{
			name: "no build info is (devel) with nothing recorded",
			info: nil,
			want: resolved(develVersion, nil, nil),
		},
		{
			name: "an empty module version is (devel)",
			info: buildOf(""),
			want: resolved(develVersion, nil, nil),
		},
		{
			name: "(devel) with no vcs settings stays (devel)",
			info: buildOf(develVersion),
			want: resolved(develVersion, nil, nil),
		},
		{
			name: "a tagged install keeps its tag and records no commit",
			info: buildOf("v1.0.0"),
			want: resolved("v1.0.0", nil, nil),
		},
		{
			name: "a clean checkout keeps its pseudo-version and records commit and clean",
			info: buildOf(pseudoVersion, revisionKey, revision, modifiedKey, "false"),
			want: resolved(pseudoVersion, new(revision), new(false)),
		},
		{
			name: "a dirty checkout keeps its pseudo-version and records commit and dirty",
			info: buildOf(pseudoVersion+"+dirty", revisionKey, revision, modifiedKey, "true"),
			want: resolved(pseudoVersion+"+dirty", new(revision), new(true)),
		},
		{
			name: "a tag with vcs settings keeps the tag and still records the facts",
			info: buildOf("v1.0.0", revisionKey, revision, modifiedKey, "true"),
			want: resolved("v1.0.0", new(revision), new(true)),
		},
		{
			name: "(devel) with a modified revision shows the short revision and +dirty",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, "true"),
			want: resolved(develVersion+" "+shortRevision+"+dirty", new(revision), new(true)),
		},
		{
			name: "(devel) with a clean revision shows the short revision alone",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, "false"),
			want: resolved(develVersion+" "+shortRevision, new(revision), new(false)),
		},
		{
			name: "an empty module version with a revision is treated as (devel)",
			info: buildOf("", revisionKey, revision, modifiedKey, "true"),
			want: resolved(develVersion+" "+shortRevision+"+dirty", new(revision), new(true)),
		},
		{
			name: "a revision shorter than 12 characters is shown whole",
			info: buildOf(develVersion, revisionKey, "260d0d29f94"),
			want: resolved(develVersion+" 260d0d29f94", new("260d0d29f94"), nil),
		},
		{
			name: "a dirty revision shorter than 12 characters is shown whole with +dirty",
			info: buildOf(develVersion, revisionKey, "260d0d29f94", modifiedKey, "true"),
			want: resolved(develVersion+" 260d0d29f94+dirty", new("260d0d29f94"), new(true)),
		},
		{
			name: "a revision of exactly 12 characters is shown whole",
			info: buildOf(develVersion, revisionKey, shortRevision),
			want: resolved(develVersion+" "+shortRevision, new(shortRevision), nil),
		},
		{
			name: "a revision of 13 characters is cut to 12 in the text and kept whole in Commit",
			info: buildOf(develVersion, revisionKey, shortRevision+"c"),
			want: resolved(develVersion+" "+shortRevision, new(shortRevision+"c"), nil),
		},
		{
			name: "a revision without a modified setting shows the revision and records modified unknown",
			info: buildOf(develVersion, revisionKey, revision),
			want: resolved(develVersion+" "+shortRevision, new(revision), nil),
		},
		{
			name: "modified 'True' is unknown, not dirty",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, "True"),
			want: resolved(develVersion+" "+shortRevision, new(revision), nil),
		},
		{
			name: "modified '1' is unknown, not dirty",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, "1"),
			want: resolved(develVersion+" "+shortRevision, new(revision), nil),
		},
		{
			name: "an empty modified is unknown, not clean",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, ""),
			want: resolved(develVersion+" "+shortRevision, new(revision), nil),
		},
		{
			name: "an empty revision is no revision, and modified is still recorded",
			info: buildOf(develVersion, revisionKey, "", modifiedKey, "true"),
			want: resolved(develVersion, nil, new(true)),
		},
		{
			name: "modified without a revision never adds +dirty",
			info: buildOf(develVersion, modifiedKey, "true"),
			want: resolved(develVersion, nil, new(true)),
		},
		{
			name: "duplicate revisions that differ are no revision",
			info: buildOf(develVersion, revisionKey, revision, revisionKey, otherRevision, modifiedKey, "true"),
			want: resolved(develVersion, nil, new(true)),
		},
		{
			name: "duplicate revisions that agree count once",
			info: buildOf(develVersion, revisionKey, revision, revisionKey, revision, modifiedKey, "true"),
			want: resolved(develVersion+" "+shortRevision+"+dirty", new(revision), new(true)),
		},
		{
			name: "duplicate modified settings that differ are unknown, so no +dirty",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, "true", modifiedKey, "false"),
			want: resolved(develVersion+" "+shortRevision, new(revision), nil),
		},
		{
			name: "duplicate modified settings that agree count once",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, "true", modifiedKey, "true"),
			want: resolved(develVersion+" "+shortRevision+"+dirty", new(revision), new(true)),
		},
		{
			name: "duplicate modified settings that agree but are malformed stay unknown",
			info: buildOf(develVersion, revisionKey, revision, modifiedKey, "TRUE", modifiedKey, "TRUE"),
			want: resolved(develVersion+" "+shortRevision, new(revision), nil),
		},
		{
			name: "a tag with conflicting duplicate revisions keeps the tag and records no commit",
			info: buildOf("v1.0.0", revisionKey, revision, revisionKey, otherRevision, modifiedKey, "false"),
			want: resolved("v1.0.0", nil, new(false)),
		},
		{
			name: "a tag with an empty revision keeps the tag and records no commit",
			info: buildOf("v1.0.0", revisionKey, "", modifiedKey, "true"),
			want: resolved("v1.0.0", nil, new(true)),
		},
		{
			name: "a tag with conflicting duplicate modified settings keeps the tag and records modified unknown",
			info: buildOf("v1.0.0", revisionKey, revision, modifiedKey, "false", modifiedKey, "true"),
			want: resolved("v1.0.0", new(revision), nil),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, version.Resolve(c.info))
		})
	}
}
