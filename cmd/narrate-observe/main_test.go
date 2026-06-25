package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderLine(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{
			name: "voiced playing",
			raw:  `{"schema":"narrate.observe.block","v":1,"block_id":"b3","order":3,"total":9,"level":2,"status":"voiced","planned_duration_ms":4200,"playing":true,"ts":"2026-06-25T17:50:01.123Z"}`,
			want: "[3/9] L2 voiced 4.2s > b3",
			ok:   true,
		},
		{
			name: "refused no audio",
			raw:  `{"schema":"narrate.observe.block","v":1,"block_id":"b7","order":7,"total":9,"level":3,"status":"refused","planned_duration_ms":1100,"playing":false,"ts":"2026-06-25T17:50:05Z"}`,
			want: "[7/9] L3 refused 1.1s . b7",
			ok:   true,
		},
		{
			name: "paused empty-audio block",
			raw:  `{"schema":"narrate.observe.block","v":1,"block_id":"b5","order":5,"total":9,"level":1,"status":"voiced","planned_duration_ms":0,"playing":false,"ts":"2026-06-25T17:50:03Z"}`,
			want: "[5/9] L1 voiced 0.0s . b5",
			ok:   true,
		},
		{
			name: "unknown future field tolerated (additive-compat)",
			raw:  `{"schema":"narrate.observe.block","v":1,"block_id":"b1","order":1,"total":2,"level":2,"status":"voiced","planned_duration_ms":500,"playing":true,"ts":"x","future_field":"ignored"}`,
			want: "[1/2] L2 voiced 0.5s > b1",
			ok:   true,
		},
		{name: "foreign schema skipped", raw: `{"schema":"some.other.event","order":1}`, ok: false},
		{name: "garbage line skipped", raw: `not json at all`, ok: false},
		{name: "empty line skipped", raw: ``, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := renderLine([]byte(tc.raw))
			if ok != tc.ok {
				t.Fatalf("ok: got %v, want %v (line %q)", ok, tc.ok, tc.raw)
			}
			if ok && got != tc.want {
				t.Errorf("render: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDrainLines_BuffersPartialTail — a complete line renders; a trailing
// partial line (no newline yet) is held back and returned, then completed on
// the next chunk. This is the partial-line-during-tail safety.
func TestDrainLines_BuffersPartialTail(t *testing.T) {
	voiced := `{"schema":"narrate.observe.block","v":1,"block_id":"b1","order":1,"total":2,"level":2,"status":"voiced","planned_duration_ms":1000,"playing":true,"ts":"x"}`
	second := `{"schema":"narrate.observe.block","v":1,"block_id":"b2","order":2,"total":2,"level":1,"status":"voiced","planned_duration_ms":0,"playing":false,"ts":"x"}`

	var out bytes.Buffer
	// First chunk: one full line + a partial (no trailing newline).
	rest := drainLines([]byte(voiced+"\n"+second[:20]), &out)
	if !strings.Contains(out.String(), "[1/2] L2 voiced 1.0s > b1") {
		t.Errorf("first complete line should render, got %q", out.String())
	}
	if strings.Contains(out.String(), "b2") {
		t.Errorf("partial second line must NOT render yet, got %q", out.String())
	}
	// Second chunk completes the partial line.
	out.Reset()
	rest = drainLines(append(rest, []byte(second[20:]+"\n")...), &out)
	if len(rest) != 0 {
		t.Errorf("buffer should be drained, got leftover %q", rest)
	}
	if !strings.Contains(out.String(), "[2/2] L1 voiced 0.0s . b2") {
		t.Errorf("completed second line should render, got %q", out.String())
	}
}

func TestResolveObserveTarget_Precedence(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.jsonl")

	t.Run("flag wins", func(t *testing.T) {
		t.Setenv("NARRATE_OBSERVE_FILE", envPath)
		got, err := resolveObserveTarget("/explicit/flag.jsonl")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "/explicit/flag.jsonl" {
			t.Errorf("flag should win, got %q", got)
		}
	})

	t.Run("env when no flag", func(t *testing.T) {
		t.Setenv("NARRATE_OBSERVE_FILE", envPath)
		got, err := resolveObserveTarget("")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != envPath {
			t.Errorf("env should be used, got %q", got)
		}
	})

	t.Run("glob miss errors with guidance", func(t *testing.T) {
		t.Setenv("NARRATE_OBSERVE_FILE", "")
		// globPattern targets /tmp; in CI there may legitimately be matches
		// from other runs, so only assert the error SHAPE when empty. Skip if
		// a real /tmp scratch happens to exist (don't false-fail).
		got, err := resolveObserveTarget("")
		if err != nil {
			if !strings.Contains(err.Error(), "no scratch file found") {
				t.Errorf("glob-miss error should guide the user, got %v", err)
			}
			return
		}
		// A match existed (newest-glob) — must be a real file path.
		if _, statErr := os.Stat(got); statErr != nil {
			t.Errorf("resolved glob target should exist: %q (%v)", got, statErr)
		}
	})
}
