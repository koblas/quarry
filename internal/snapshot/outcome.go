package snapshot

import (
	"fmt"
	"path/filepath"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/sqlschema"
)

// MismatchError is Sync's refusal for a schema check that finds a table or
// column the reference names and the bundle lacks: its message excludes
// the "quarry: " prefix a caller adds before printing it to stderr.
type MismatchError struct {
	msg string
}

// Error returns the refusal's message verbatim.
func (e MismatchError) Error() string { return e.msg }

// mismatchError renders the mismatch refusal: bundlePath is named by its
// base name, snapshotPath is ~-abbreviated against home.
func mismatchError(home, bundlePath, snapshotPath string, info SchemaInfo) MismatchError {
	return MismatchError{msg: fmt.Sprintf(
		"schema check failed: %s is missing %s that the schema reference expects; "+
			"the snapshot is kept at %s and the diff is in its .json manifest; "+
			"quarry cannot import this file until its schema reference is updated",
		filepath.Base(bundlePath),
		sqlschema.CountPhrase(len(info.MissingTables), len(info.MissingColumns)),
		homepath.Abbreviate(home, snapshotPath))}
}

// fromMismatchError renders the --from mismatch refusal: the snapshot is
// named by its ID, bundlePath by its base name.
func fromMismatchError(snapshotPath, bundlePath string, info SchemaInfo) MismatchError {
	return MismatchError{msg: fmt.Sprintf(
		"schema check failed: snapshot %s of %s is missing %s that the schema reference expects; "+
			"quarry cannot import it until its schema reference is updated",
		snapshotID(snapshotPath), filepath.Base(bundlePath),
		sqlschema.CountPhrase(len(info.MissingTables), len(info.MissingColumns)))}
}

// extrasWarningText renders the extras warning's body: bundlePath and
// manifestPath are both named by their base name. The verb and pronoun
// agree with the singular only when exactly one table or column, in
// total, is unexpected.
func extrasWarningText(bundlePath, manifestPath string, info SchemaInfo) string {
	verb, pronoun := "are", "them"
	if len(info.UnexpectedTables)+len(info.UnexpectedColumns) == 1 {
		verb, pronoun = "is", "it"
	}
	return fmt.Sprintf(
		"%s has %s that %s not in the schema reference; quarry ignores %s (listed in %s)",
		filepath.Base(bundlePath),
		sqlschema.CountPhrase(len(info.UnexpectedTables), len(info.UnexpectedColumns)),
		verb, pronoun,
		filepath.Base(manifestPath))
}

// hasOnlyExtras reports whether info has unexpected tables or columns and
// no missing ones: the extras-warning condition, and MismatchError's
// opposite.
func hasOnlyExtras(info SchemaInfo) bool {
	return info.Verified && info.HasExtras()
}
