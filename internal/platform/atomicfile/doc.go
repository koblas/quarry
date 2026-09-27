// Package atomicfile creates files that never silently overwrite an existing
// one: an exclusive create for a partial file, and a no-clobber commit that
// moves it into its final name.
package atomicfile
