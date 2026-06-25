// x/term backing for the listen-path raw-mode seam (issue #83).
//
// enterRaw in listen.go talks to the package-level termMakeRaw / termRestore
// seams so the controller is unit-testable without a real tty. This file wires
// those seams to golang.org/x/term in production. Pulled into its own file so
// listen_test.go can swap the seams without dragging x/term into the test's
// expectations.
package main

import (
	"fmt"

	"golang.org/x/term"
)

// defaultMakeRaw puts the terminal at fd into raw mode via x/term, returning
// the prior state (as the seam's opaque rawRestoreState) so defaultRestore can
// undo it. The concrete *term.State is hidden behind rawRestoreState so the
// rest of the package never imports x/term directly.
func defaultMakeRaw(fd int) (rawRestoreState, error) {
	st, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("term.MakeRaw fd=%d: %w", fd, err)
	}
	return st, nil
}

// defaultRestore restores the terminal at fd from the state captured by
// defaultMakeRaw. A nil or wrong-typed state is a programmer error in the seam
// wiring, surfaced as an error rather than a panic.
func defaultRestore(fd int, st rawRestoreState) error {
	s, ok := st.(*term.State)
	if !ok || s == nil {
		return fmt.Errorf("restore fd=%d: invalid raw state %T", fd, st)
	}
	if err := term.Restore(fd, s); err != nil {
		return fmt.Errorf("term.Restore fd=%d: %w", fd, err)
	}
	return nil
}

// isTerminal reports whether fd is a real terminal. The listen path fail-fasts
// when stdin is piped/redirected (not a tty) — raw-mode single-key control is
// meaningless without an interactive terminal, so we refuse with an honest
// message rather than silently hanging on a non-tty read.
func isTerminal(fd int) bool {
	return term.IsTerminal(fd)
}
