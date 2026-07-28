// Command plandump is a plan-only composition root: it reads a document through
// the file adapter, runs the PURE planner (nil intelligence, no renderer, no
// sink, no audio), and writes plan.json to stdout.
//
// Why it exists (#164): the machine-checkable AC3 half A (Segment.Text
// correctness per structured class) and the gso-perf-baseline block-text feed
// both need the engine-neutral spoken text of a document WITHOUT spawning the
// GPT-SoVITS worker or producing any audio. cmd/narrate always renders (needs a
// renderer + sink + a subprocess and, for cool-jahns-gso, the GSO worker); this
// helper stops after the planner. The plan it emits is identical to what
// cmd/narrate would produce on the default CLI path, because it uses the same
// file adapter and the same planner.Plan(..., nil-intelligence) call the
// pipeline uses (pipeline/pipeline.go) — Segment.Text is produced in the planner
// and is engine-neutral (CLAUDE.md: "Voicing happens in the planner, not the
// renderer"), so the dumped plan.json speaks the same words any engine would.
//
// It is a dev/verify utility, not part of the narrate/MCP/server surface: no new
// flag was added to those binaries (#164 out-of-scope), the planner/plan packages
// are untouched, and this root only composes existing edges (CLAUDE.md: only
// pipeline/ and cmd/ know concrete edges).
//
// Usage:
//
//	go run ./cmd/plandump --file docs/samples/sample.md          # plan.json → stdout
//	go run ./cmd/plandump --file docs/samples/gso-g2p-coverage.md --level 1
//
// Exit codes: 0 on success; 1 on any adapter/planner/encode error; 2 on a flag
// error (missing --file).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	fileadapter "github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/planner"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, errFlag) {
			fmt.Fprintln(os.Stderr, "plandump:", err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "plandump:", err)
		os.Exit(1)
	}
}

// errFlag — sentinel so a bad invocation exits 2 (argument error), distinct from
// a real adapter/planner failure (exit 1).
var errFlag = errors.New("flag error")

func run() error {
	var (
		file  string
		level int
	)
	flag.StringVar(&file, "file", "", "path to the document to plan (required)")
	flag.IntVar(&level, "level", 1, "requested level: 1 (gist) | 2 (summary) | 3 (detail)")
	flag.Parse()

	if file == "" {
		return fmt.Errorf("%w: --file is required", errFlag)
	}
	lvl := plan.Level(level)
	if !lvl.IsValid() {
		return fmt.Errorf("%w: --level must be 1, 2, or 3 (got %d)", errFlag, level)
	}

	ctx := context.Background()

	// Same file adapter + planner.Plan(nil intelligence) the default CLI path uses
	// (pipeline/pipeline.go). No renderer, no sink, no audio, no GSO worker.
	doc, err := fileadapter.New().Read(ctx, plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  file,
	})
	if err != nil {
		return fmt.Errorf("adapter: %w", err)
	}

	p, err := planner.Plan(ctx, doc, planner.Request{Level: lvl}, nil)
	if err != nil {
		return fmt.Errorf("planner: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return fmt.Errorf("encode plan.json: %w", err)
	}
	return nil
}
