// Package document builds JSON documents shared by the command line and the
// MCP server, so each field has one owner and the two surfaces never disagree.
//
// Builders take values and return values: they never encode, and a warnings
// list is always passed in and always comes out as an array, never null. The
// caller chooses the encoding (indented for the command line, compact for MCP).
//
// The Spending, CashFlow, Recurring and Anomalies warnings composers return
// unprefixed lines, never nil; their word argument names the command in the
// left-out-of-reports lines and nowhere else.
package document
