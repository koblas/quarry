// Package replacefile writes a file so that its path holds either the whole
// old contents or the whole new contents at every instant: it writes a
// temporary file beside the target and renames it over.
//
// Unlike package atomicfile it replaces an existing file; it never inspects
// what is at the path, so a caller that must not replace a symbolic link
// checks first.
package replacefile
