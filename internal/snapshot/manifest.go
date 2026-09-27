package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Manifest is quarry's on-disk manifest and --json document (BR-10): the
// same bytes are written to <snapshot>.json and printed to stdout.
type Manifest struct {
	Snapshot SnapshotInfo `json:"snapshot"`
	Schema   SchemaInfo   `json:"schema"`
	Warnings []string     `json:"warnings"`
}

// SnapshotInfo describes the snapshot file itself.
type SnapshotInfo struct {
	Path     string `json:"path"`
	Manifest string `json:"manifest"`
	Source   string `json:"source"`
	TakenAt  string `json:"taken_at"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Accounts int    `json:"accounts"`
}

// SchemaInfo reports the snapshot's schema against the reference (BR-6,
// BR-7). Verified is true exactly when MissingTables and MissingColumns are
// both empty (BR-9); extra tables or columns do not affect it.
type SchemaInfo struct {
	Reference            string      `json:"reference"`
	Verified             bool        `json:"verified"`
	Fingerprint          string      `json:"fingerprint"`
	ReferenceFingerprint string      `json:"reference_fingerprint"`
	MissingTables        []string    `json:"missing_tables"`
	MissingColumns       []ColumnRef `json:"missing_columns"`
	UnexpectedTables     []string    `json:"unexpected_tables"`
	UnexpectedColumns    []ColumnRef `json:"unexpected_columns"`

	// ReferenceTables and ReferenceColumns count the scoped reference
	// schema's tables and columns, for the CLI's human Schema line. They
	// are not part of the --json document.
	ReferenceTables  int `json:"-"`
	ReferenceColumns int `json:"-"`
}

// ColumnRef names one column of one table in the --json document.
type ColumnRef struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

// Encode returns m as the exact bytes written to disk and printed with
// --json: 2-space indented JSON with a trailing newline. It is the only
// encoder for Manifest, so the file on disk and stdout are always identical
// (BR-10).
func (m Manifest) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		// unreachable: every Manifest field is a string, bool, int, int64, slice or struct of those; none can fail JSON encoding.
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return buf.Bytes(), nil
}
