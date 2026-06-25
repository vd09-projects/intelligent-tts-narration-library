package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
)

// frozenClock pins observeNow for a test and restores it after.
func frozenClock(t *testing.T, ts string) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t.Fatalf("frozenClock: bad ts %q: %v", ts, err)
	}
	orig := observeNow
	observeNow = func() time.Time { return parsed.UTC() }
	t.Cleanup(func() { observeNow = orig })
}

func sampleProgress() ephemeral.BlockProgress {
	return ephemeral.BlockProgress{
		Timing:  plan.BlockTiming{BlockID: "b3", StartMs: 1000, EndMs: 5200, AudioRef: "b3.wav"},
		Level:   plan.L2,
		Status:  plan.StatusVoiced,
		Order:   3,
		Total:   9,
		Playing: true,
	}
}

// TestMarshalBlockEvent_ByteExactShape — frozen clock pins the wire bytes so a
// field rename / reorder / type change is caught. Also asserts the trailing
// newline (one-Write atomicity) and that NO source / spoken-text field leaks
// onto the wire (S11 secret-leak avoidance).
func TestMarshalBlockEvent_ByteExactShape(t *testing.T) {
	frozenClock(t, "2026-06-25T17:50:01.123Z")

	got, err := marshalBlockEvent(sampleProgress())
	if err != nil {
		t.Fatalf("marshalBlockEvent: %v", err)
	}

	want := `{"schema":"narrate.observe.block","v":1,"block_id":"b3","order":3,"total":9,"level":2,"status":"voiced","planned_duration_ms":4200,"playing":true,"ts":"2026-06-25T17:50:01.123Z"}` + "\n"
	if string(got) != want {
		t.Errorf("wire bytes drifted:\n got: %s\nwant: %s", got, want)
	}
	if !bytes.HasSuffix(got, []byte("\n")) {
		t.Error("line must be newline-terminated (single-Write atomicity)")
	}
	// S11: no source / spoken text on the wire, ever.
	for _, banned := range []string{"spoken", "source", "raw_excerpt", "segment"} {
		if bytes.Contains(got, []byte(banned)) {
			t.Errorf("secret-leak guard: wire line must not contain %q, got %s", banned, got)
		}
	}
}

// TestMarshalBlockEvent_RoundTrips — the line decodes back to the same
// structural values (the observer reader's contract).
func TestMarshalBlockEvent_RoundTrips(t *testing.T) {
	frozenClock(t, "2026-06-25T17:50:01.123Z")
	line, err := marshalBlockEvent(sampleProgress())
	if err != nil {
		t.Fatalf("marshalBlockEvent: %v", err)
	}
	var ev blockEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	want := blockEvent{
		Schema: observeSchemaName, V: observeSchemaVersion, BlockID: "b3",
		Order: 3, Total: 9, Level: 2, Status: "voiced",
		PlannedDurationMs: 4200, Playing: true, TS: "2026-06-25T17:50:01.123Z",
	}
	if ev != want {
		t.Errorf("round-trip: got %+v, want %+v", ev, want)
	}
}

func TestResolveObservePath_Precedence(t *testing.T) {
	cases := []struct {
		name         string
		file         string // NARRATE_OBSERVE_FILE
		toggle       string // NARRATE_OBSERVE
		wantPath     string
		wantAutoTemp bool
	}{
		{"file wins over toggle", "/tmp/explicit.jsonl", "1", "/tmp/explicit.jsonl", false},
		{"file alone", "/tmp/x.jsonl", "", "/tmp/x.jsonl", false},
		{"toggle truthy → auto-temp", "", "true", "", true},
		{"toggle on", "", "on", "", true},
		{"toggle off (0)", "", "0", "", false},
		{"toggle off (false)", "", "false", "", false},
		{"toggle unset", "", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envObserveFile, tc.file)
			t.Setenv(envObserve, tc.toggle)
			gotPath, gotAuto := resolveObservePath()
			if gotPath != tc.wantPath || gotAuto != tc.wantAutoTemp {
				t.Errorf("got (%q, %v), want (%q, %v)", gotPath, gotAuto, tc.wantPath, tc.wantAutoTemp)
			}
		})
	}
}

// TestNewBlockObserver_WritesJSONL — the explicit-path branch opens the file,
// announces it once, and each emit appends one JSONL line.
func TestNewBlockObserver_WritesJSONL(t *testing.T) {
	frozenClock(t, "2026-06-25T17:50:01.123Z")
	path := filepath.Join(t.TempDir(), "scratch.jsonl")
	t.Setenv(envObserveFile, path)
	t.Setenv(envObserve, "")

	var stderr bytes.Buffer
	obs, closeObs := newBlockObserver(&stderr)
	if obs == nil {
		t.Fatal("want a live observer for an openable NARRATE_OBSERVE_FILE path")
	}
	obs(sampleProgress())
	// A pause / empty-audio block — the realistic Playing:false case.
	obs(ephemeral.BlockProgress{Timing: plan.BlockTiming{BlockID: "b4"}, Level: plan.L1, Status: plan.StatusVoiced, Order: 4, Total: 9, Playing: false})
	closeObs()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scratch: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 JSONL lines, got %d: %q", len(lines), data)
	}
	// 0600 owner-only (secret-leak insurance).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat scratch: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("scratch perm: want 0600, got %o", perm)
	}
	if !strings.Contains(stderr.String(), "writing "+path) {
		t.Errorf("expected one announce line naming the path, got %q", stderr.String())
	}
}

// TestNewBlockObserver_OpenFailure_OneStderr_NeverErrors — a bad
// NARRATE_OBSERVE_FILE path disables observation with EXACTLY one STDERR line
// and a nil observer (the sink's off state); it never returns / propagates an
// error (the listen path must not break on a fat-fingered env var).
func TestNewBlockObserver_OpenFailure_OneStderr_NeverErrors(t *testing.T) {
	t.Setenv(envObserveFile, "/nonexistent-dir-xyz/scratch.jsonl")
	t.Setenv(envObserve, "")

	var stderr bytes.Buffer
	obs, closeObs := newBlockObserver(&stderr)
	defer closeObs()
	if obs != nil {
		t.Error("open failure must yield a nil (off) observer, not a live one")
	}
	if n := strings.Count(stderr.String(), "\n"); n != 1 {
		t.Errorf("open failure must emit exactly ONE STDERR line, got %d: %q", n, stderr.String())
	}
	if !strings.Contains(stderr.String(), "disabled") {
		t.Errorf("STDERR line should say disabled, got %q", stderr.String())
	}
}

// TestNewBlockObserver_WriteFailure_OneStderr — once the scratch handle is
// broken (closed under us), every emit's Write fails, but only the FIRST emits
// a STDERR line; the rest go silent (no per-block spam) and none panics.
func TestNewBlockObserver_WriteFailure_OneStderr(t *testing.T) {
	frozenClock(t, "2026-06-25T17:50:01.123Z")
	path := filepath.Join(t.TempDir(), "scratch.jsonl")
	t.Setenv(envObserveFile, path)
	t.Setenv(envObserve, "")

	var stderr bytes.Buffer
	obs, closeObs := newBlockObserver(&stderr)
	if obs == nil {
		t.Fatal("want a live observer")
	}
	// Drop the announce line so we count only write-failure output.
	stderr.Reset()
	// Break the handle: closeObs closes the underlying *os.File, so every
	// subsequent Write returns os.ErrClosed.
	closeObs()

	obs(sampleProgress())
	obs(sampleProgress())
	obs(sampleProgress())

	if n := strings.Count(stderr.String(), "\n"); n != 1 {
		t.Errorf("write failure must emit exactly ONE STDERR line across many emits, got %d: %q", n, stderr.String())
	}
}

// TestNewBlockObserver_Off — neither env var set: observation is off, nil
// observer, no STDERR noise.
func TestNewBlockObserver_Off(t *testing.T) {
	t.Setenv(envObserveFile, "")
	t.Setenv(envObserve, "")
	var stderr bytes.Buffer
	obs, closeObs := newBlockObserver(&stderr)
	defer closeObs()
	if obs != nil {
		t.Error("off: want nil observer")
	}
	if stderr.Len() != 0 {
		t.Errorf("off: want no STDERR, got %q", stderr.String())
	}
}

// TestRunSpeak_ResponseByteIdentical_ObserveOffAndOn — the issue #81 B3
// counter-metric. Enabling the Channel-2 observer must NOT change the speak
// RESPONSE: only the side scratch file differs. Asserts bytes.Equal on the
// marshaled speakResponse for (i) two observe-OFF runs and (ii) observe-OFF vs
// observe-ON. Receipt.OutDir is the one legitimately non-deterministic field
// (a random per-call temp dir, deleted on return, debug-only per the
// speakReceipt docstring) so it is zeroed before comparison — every OTHER byte
// must be identical. Uses the stub narrator (newPipeline seam) so no Kokoro.
func TestRunSpeak_ResponseByteIdentical_ObserveOffAndOn(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var + env.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 2, TotalDurationMs: 3300}}
	restore := withStubPipeline(t, stub)
	defer restore()

	src, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = src.Close()
	args := speakArgs{Source: src.Name(), Level: 2}

	marshalResp := func(t *testing.T) []byte {
		t.Helper()
		resp, err := runSpeak(context.Background(), args)
		if err != nil {
			t.Fatalf("runSpeak: %v", err)
		}
		resp.Receipt.OutDir = "" // normalize the one random field.
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal resp: %v", err)
		}
		return b
	}

	// Baseline + control: two OFF runs must be byte-identical modulo OutDir.
	t.Setenv(envObserveFile, "")
	t.Setenv(envObserve, "")
	baseline := marshalResp(t)
	control := marshalResp(t)
	if !bytes.Equal(baseline, control) {
		t.Fatalf("two observe-OFF runs differ (non-determinism beyond OutDir):\n %s\n %s", baseline, control)
	}

	// ON direction: response still byte-identical to the OFF baseline. This is
	// the decoupling proof — the observer only writes the side file.
	t.Setenv(envObserveFile, filepath.Join(t.TempDir(), "scratch.jsonl"))
	on := marshalResp(t)
	if !bytes.Equal(baseline, on) {
		t.Errorf("observe-ON changed the speak response (must be decoupled):\noff: %s\n on: %s", baseline, on)
	}
}
