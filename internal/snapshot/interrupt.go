package snapshot

import "context"

// InterruptedRefusal reports that the run was interrupted before anything
// could be committed, with every partial file it created removed. cmd/quarry
// also reports it when loading the reference schema fails against an
// already-cancelled context, ahead of ever calling Sync.
func InterruptedRefusal() error {
	return RefusalError{msg: "sync interrupted; nothing was kept; run quarry sync again"}
}

// failureOutcome reclassifies refusal as InterruptedRefusal once ctx has
// already ended, so every pre-commit failure site can route through it.
func failureOutcome(ctx context.Context, refusal error) error {
	if ctx.Err() != nil {
		return InterruptedRefusal()
	}
	return refusal
}
