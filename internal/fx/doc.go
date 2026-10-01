// Package fx fetches Bank of Canada USD/CAD exchange rates for a store build.
// *Server satisfies the store's rates port; Source is fx's own port to the
// rate publisher.
//
// The publisher is the Bank of Canada Valet web service,
// https://www.bankofcanada.ca/valet/observations/<series>/json with the
// start_date and end_date query parameters. Two series are read: FXUSDCAD
// (the current series) and IEXE0101 (the legacy series, used only for days
// before FXUSDCAD's first published day). The cutover is the first FXUSDCAD
// observation in the answer, never a constant; a day in both series takes
// FXUSDCAD's rate.
package fx
