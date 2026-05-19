// Package surge exposes the reusable download library behind the Surge CLI and TUI.
//
// The package is intentionally UI-agnostic: it owns download services, worker
// pools, events, and data models, but it does not import Cobra, Bubble Tea, or
// any terminal-specific code.
package surge
