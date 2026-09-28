package importer

// UnmappableError reports a value Import cannot map to quarry's schema: an
// unsupported currency or account type, a source value quarry does not
// model, or a required field the snapshot leaves empty. Reason is the
// caller-facing text, rendered verbatim by the S4 refusal.
type UnmappableError struct {
	Reason string
}

// Error returns Reason.
func (e *UnmappableError) Error() string {
	return e.Reason
}
