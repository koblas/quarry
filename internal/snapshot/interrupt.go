package snapshot

import "context"

// InterruptedRefusal reports that the run was interrupted before anything
// could be committed, with every partial file it created removed.
func InterruptedRefusal() error {
	return RefusalError{msg: "sync interrupted; nothing was kept; run quarry sync again"}
}

// FailureOutcome reclassifies refusal as InterruptedRefusal once ctx has
// already ended, so every pre-commit failure site, inside Sync or ahead of
// it, routes through the same decision.
func FailureOutcome(ctx context.Context, refusal error) error {
	if ctx.Err() != nil {
		return InterruptedRefusal()
	}
	return refusal
}
