// Package document builds JSON documents shared by the command line and the
// MCP server, so each field has one owner and the two surfaces never disagree.
//
// Builders take values and return values: they never encode, and a warnings
// list is always passed in and always comes out as an array, never null. The
// caller chooses the encoding (indented for the command line, compact for MCP).
package document
