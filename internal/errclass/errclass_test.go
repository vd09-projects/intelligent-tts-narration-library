package errclass

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

// TestClassify exercises every ladder branch — cancel, the fs caller sentinels,
// wrap-traversal, nil, and the default internal arm. errclass owns CATEGORY
// only — the per-root wire mapping (text prefixes / HTTP status) is tested by
// the cmd behavior-oracle tests, not here. Adapter sentinels (mcpsampling.Err*)
// are deliberately NOT rowed here: they land on the default ClassInternal arm
// (no dedicated branch), and the cmd MCP/server tests assert their end-to-end
// wire behaviour (#58).
func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   error
		want Class
	}{
		// Cancel bucket (precedence 1).
		{"context.Canceled", context.Canceled, ClassCancelled},
		{"context.DeadlineExceeded", context.DeadlineExceeded, ClassCancelled},

		// Caller bucket (precedence 2-3).
		{"fs.ErrNotExist", fs.ErrNotExist, ClassCaller},
		{"fs.ErrPermission", fs.ErrPermission, ClassCaller},
		// Wrapped error proves errors.Is traversal through a %w chain.
		{"wrapped fs.ErrNotExist", fmt.Errorf("ctx: %w", fs.ErrNotExist), ClassCaller},

		// Default / safe-fallback (precedence 4). Any unrecognised error —
		// including adapter sentinels like mcpsampling's — lands here.
		{"unknown error -> internal", errors.New("disk on fire"), ClassInternal},
		// nil never reaches Classify in production (both roots guard non-nil);
		// documented defined-but-unreached behaviour.
		{"nil -> internal (defined, unreached in prod)", nil, ClassInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(tc.in); got != tc.want {
				t.Errorf("Classify(%v) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestClassPrecedence pins the order-significant ladder: cancel beats caller
// beats internal when sentinels are joined in one error chain.
func TestClassPrecedence(t *testing.T) {
	t.Parallel()
	// A chain carrying both a cancel and a caller sentinel must classify as
	// cancel (precedence 1 over 2).
	chain := fmt.Errorf("wrap: %w", fmt.Errorf("%w / %w", context.Canceled, fs.ErrNotExist))
	if got := Classify(chain); got != ClassCancelled {
		t.Errorf("cancel must win over caller: got %s, want ClassCancelled", got)
	}
}

func TestClassString(t *testing.T) {
	t.Parallel()
	cases := map[Class]string{
		ClassInternal:  "ClassInternal",
		ClassCaller:    "ClassCaller",
		ClassCancelled: "ClassCancelled",
		Class(99):      "Class(99)",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("Class(%d).String() = %q, want %q", int(c), got, want)
		}
	}
}
