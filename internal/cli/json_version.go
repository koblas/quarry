package cli

import "github.com/koblas/quarry/internal/version"

// versionDocument is version's --json document. Commit and Modified are null
// when the build did not record them readably.
type versionDocument struct {
	Version  string  `json:"version"`
	Commit   *string `json:"commit"`
	Modified *bool   `json:"modified"`
	Go       string  `json:"go"`
}

// renderVersionJSON renders info as version's --json document, encoded like every other: 2-space indent, trailing newline.
func renderVersionJSON(info version.Info) ([]byte, error) {
	return marshalDocument(versionDocument{Version: info.Version, Commit: info.Commit, Modified: info.Modified, Go: info.Go})
}
