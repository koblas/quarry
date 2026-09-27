// Package v9 embeds quarry's reference schema for Quicken Classic for Mac
// v9's Core Data database and derives it into a comparable form. It knows
// nothing about snapshotting or scoping; the snapshot package consumes it
// and applies its own scope before comparing.
package v9
