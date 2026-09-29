package store

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
// (OpenFaultOtherFormat only) is the import run's snapshot, "" when
// unreadable; Reason (OpenFaultOther only) is the fault's first line with
// the store named by Path. Err is the underlying fault.
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
