package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Manifest is quarry's on-disk manifest, written next to each snapshot as
// <snapshot>.json.
type Manifest struct {
	Snapshot Info       `json:"snapshot"`
	Schema   SchemaInfo `json:"schema"`
	Warnings []string   `json:"warnings"`
}

// Info describes the snapshot file itself.
type Info struct {
	Path     string `json:"path"`
	Manifest string `json:"manifest"`
	Source   string `json:"source"`
	TakenAt  string `json:"taken_at"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Accounts int    `json:"accounts"`
}

// SchemaInfo reports the snapshot's schema against the reference. Verified
// is true exactly when MissingTables and MissingColumns are both empty;
// extra tables or columns do not affect it.
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

// HasExtras reports whether s has any table or column the reference does
// not name.
func (s SchemaInfo) HasExtras() bool {
	return len(s.UnexpectedTables) > 0 || len(s.UnexpectedColumns) > 0
}

// ColumnRef names one column of one table in the --json document.
type ColumnRef struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

// Encode returns m as the exact bytes written to disk: 2-space indented
// JSON with a trailing newline. It is the only encoder for Manifest.
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
