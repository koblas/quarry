// Package document builds the JSON documents quarry prints with --json and
// returns over MCP, so the command line and the MCP server share one owner
// for every field and never disagree.
//
// Builders take values and return values: they never encode, and a warnings
// list is always passed in and always comes out as an array, never null. The
// caller chooses the encoding (indented for the command line, compact for MCP).
package document
