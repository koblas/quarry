package store

import "strings"

// OpenFault is why a read could not open the store.
type OpenFault int

// The faults a read's open classifies; OpenFaultOther is any fault not named here.
const (
	OpenFaultOther OpenFault = iota
	OpenFaultMissing
	OpenFaultOtherFormat
	OpenFaultNotDuckDB
	OpenFaultPermission
	OpenFaultLocked
)

// OpenError is a read refused while opening the store at Path. SnapshotPath
// (OpenFaultOtherFormat only) is the highest-id import run's snapshot, "" when
// unreadable; Reason (OpenFaultOther only) is the fault's first line with the
// store named by Path, or a ready phrase where the fault is in the store's import
// history. Err is the underlying fault, not user copy: print UnreadableReason.
type OpenError struct {
	Fault        OpenFault
	Path         string
	SnapshotPath string
	Reason       string
	Err          error
}

func (e *OpenError) Error() string {
	if e.Err == nil {
		return "open store " + e.Path
	}
	return "open store " + e.Path + ": " + e.Err.Error()
}

func (e *OpenError) Unwrap() error { return e.Err }

// unreadableReasons is the reason phrase of each fault that fixes it; the rest carry Reason.
var unreadableReasons = map[OpenFault]string{
	OpenFaultNotDuckDB:  "the file is not a DuckDB database",
	OpenFaultPermission: "permission denied",
	OpenFaultLocked:     "another program has it open for writing",
}

// UnreadableReason is why the store could not be read, as a bare phrase without a prefix or
// remedy, naming the store as at (a display form of Path). A fault with no fixed phrase gives
// Reason with each Path replaced by at, which is empty for OpenFaultMissing and OpenFaultOtherFormat.
func (e *OpenError) UnreadableReason(at string) string {
	if reason, ok := unreadableReasons[e.Fault]; ok {
		return reason
	}
	if e.Path == "" {
		return e.Reason
	}
	return strings.ReplaceAll(e.Reason, e.Path, at)
}
