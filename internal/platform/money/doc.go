// Package money converts exact amounts between CAD and USD at a stored
// exchange rate, rounding half away from zero, so Go code and the store's SQL
// views agree on every cent.
package money
