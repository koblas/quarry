// Package toolrun runs an external command and reports how it ended, in the
// shape the features that drive other tools (such as claudeplugin's Runner)
// consume: stdout, combined output, an exit status, and an error only when the command
// could not run to its own exit.
package toolrun
