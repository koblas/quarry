// Package report answers quarry's read commands from the store sync built:
// it opens the store through its Store port and returns driver-free values
// for the cli package to render, or the one-line refusal copy when the store
// cannot be read. It never writes the store and never reads Quicken.
package report
