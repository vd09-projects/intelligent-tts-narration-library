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

	resp, err := runSpeak(context.Background(), speakArgs{
		Source: docPath,
		Level:  1,
		Sink:   "ephemeral",
		Gender: "female",
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
