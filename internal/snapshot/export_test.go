package snapshot

// NewSQLiteSource exposes the production Source adapter to tests outside
// this package. Fault tests wrap it to inject a single failing Source
// method while the rest of Sync's pipeline runs for real.
var NewSQLiteSource = newSQLiteSource

// NewDirDestination exposes the production Destination adapter to tests
// outside this package. Fault tests wrap it to inject a single failing
// Destination method while the rest of Sync's pipeline runs for real.
var NewDirDestination = newDirDestination
