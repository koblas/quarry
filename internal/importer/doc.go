// Package importer maps a Quicken Classic for Mac v9 snapshot into quarry's
// own schema and hands the mapped rows to a Store to build. It is the only
// package that knows Quicken's schema; the Store it writes to knows only
// quarry's.
package importer
