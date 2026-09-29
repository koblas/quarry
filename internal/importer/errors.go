package importer

import "github.com/koblas/quarry/internal/store"

// UnmappableError reports a value Import cannot map to quarry's schema: an
// unsupported currency or account type, a source value quarry does not
// model, or a required field the snapshot leaves empty. Reason is the
// caller-facing text, rendered verbatim by the unmappable-value refusal.
type UnmappableError struct {
	Reason string
}

// Error returns Reason.
func (e *UnmappableError) Error() string {
	return e.Reason
}

// Is reports whether target is store.ErrUnmappable, without adding it to
// the Unwrap chain whose innermost error is the reason.
func (e *UnmappableError) Is(target error) bool {
	return target == store.ErrUnmappable
}
