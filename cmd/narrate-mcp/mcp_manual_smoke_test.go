//go:build manual

// Manual smoke for cmd/narrate-mcp. Drives the production runSpeak (real
// pipeline + real Kokoro subprocess + real afplay) against the canonical
// demo doc. Gated by //go:build manual so `go test ./...` skips it —
// run with `make test-mcp-manual`.
//
// Pattern matches pipeline/pipeline_manual_smoke_test.go and the sink
// ephemeral_smoke_test.go: build tag over env-var gating, so skipped
// state is visible in `go test` output.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSpeakManualSmoke(t *testing.T) {
	// Resolve the canonical demo doc relative to the project root. The
	// test binary runs from cmd/narrate-mcp/, so we walk up two levels.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	docPath, err := filepath.Abs(filepath.Join(repoRoot, "docs", "samples", "sample.md"))
	if err != nil {
		t.Fatalf("resolve sample doc: %v", err)
	}

	// #164: NARRATE_SMOKE_SOURCE overrides the canonical demo doc so the MCP
	// by-ear verify can target a short doc. sample.md's long verbatim-prose
	// blocks synth for minutes under a slow peer engine (GSO), so a short doc
	// keeps the by-ear loop fast. Empty keeps sample.md, so make test-mcp-manual
	// is unchanged. Relative paths resolve against the repo root (the test
	// chdirs there below), mirroring the sample.md default.
	if src := os.Getenv("NARRATE_SMOKE_SOURCE"); src != "" {
		if filepath.IsAbs(src) {
			docPath = src
		} else if docPath, err = filepath.Abs(filepath.Join(repoRoot, src)); err != nil {
			t.Fatalf("resolve NARRATE_SMOKE_SOURCE: %v", err)
		}
	}

	// runSpeak builds its pipeline with sherpa's default BinaryPath
	// ("./scripts/kokoro"), resolved against the working directory. The test
	// binary runs from cmd/narrate-mcp/, so chdir to the repo root makes the
	// wrapper script (and its venv + models) resolvable.
	t.Chdir(repoRoot)

	// #147: NARRATE_SMOKE_VOICE drives the by-ear MCP verify through a named
	// roster voice (e.g. cool-jahns → the RVC decorator, 40 kHz via afplay). It
	// takes precedence over gender (effectiveVoice); empty keeps the legacy
	// female-Kokoro path so `make test-mcp-manual` is unchanged.
	voice := os.Getenv("NARRATE_SMOKE_VOICE")
	if voice != "" {
		t.Logf("NARRATE_SMOKE_VOICE=%s — narrating through the roster voice (RVC voices repaint via the worker)", voice)
	}

	resp, err := runSpeak(context.Background(), speakArgs{
		Source: docPath,
		Level:  1,
		Sink:   "ephemeral",
		Gender: "female",
		Voice:  voice,
	})
	if err != nil {
		t.Fatalf("runSpeak: %v", err)
	}
	if resp.Receipt.BlocksPlayed <= 0 {
		t.Errorf("blocks_played should be > 0, got %d", resp.Receipt.BlocksPlayed)
	}
	if resp.Receipt.TotalDurationMs <= 0 {
		t.Errorf("total_duration_ms should be > 0, got %d", resp.Receipt.TotalDurationMs)
	}
	if resp.Receipt.OutDir == "" {
		t.Error("out_dir should be non-empty")
	}

	// Human-readable for the manual smoke operator.
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	t.Logf("speak response:\n%s", pretty)
}
