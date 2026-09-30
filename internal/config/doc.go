// Package config reads quarry's optional config file, config.toml, from the
// directory that holds its store. A missing or empty file yields defaults;
// a malformed or invalid one is refused before any command touches Quicken.
package config
