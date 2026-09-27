package sqlschema

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Fingerprint returns "sha256:<hex>" over the UTF-8 bytes of "TABLE.COLUMN\n"
// for every table and column in s, sorted bytewise. Two schemas with the
// same table and column names, in any order, produce the same fingerprint.
func Fingerprint(s Schema) string {
	lines := make([]string, 0, len(s))
	for table, cols := range s {
		for _, col := range cols {
			lines = append(lines, table+"."+col+"\n")
		}
	}
	sort.Strings(lines)

	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
