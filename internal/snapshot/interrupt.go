package snapshot

import "context"

// InterruptedRefusal is I1: the run was interrupted (SIGINT/SIGTERM) before
// anything could be committed, and every partial file it created was
// removed. cmd/quarry reports it too when its own pre-Sync setup — loading
// the reference schema — fails against an already-cancelled context.
func InterruptedRefusal() error {
	return RefusalError{msg: "sync interrupted; nothing was kept; run quarry sync again"}
}

// failureOutcome classifies a pre-commit Sync failure: I1 once ctx has
// already ended, otherwise refusal unchanged. Every fallible step ahead of
// the commit sequence's own single ctx check routes its failure through
// this, so a cancelled run reports I1 instead of whatever refusal the
// step's own error would otherwise classify to.
func failureOutcome(ctx context.Context, refusal error) error {
	if ctx.Err() != nil {
		return InterruptedRefusal()
	}
	return refusal
}
