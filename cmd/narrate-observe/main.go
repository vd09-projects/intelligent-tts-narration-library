// Command narrate-observe — the read-only Channel-2 observer (issue #81,
// ADR #77 D5 Channel 2). Launch it in a SECOND terminal; it tail -f's the
// ephemeral JSONL scratch file cmd/narrate-mcp's speak handler writes one
// line per block to, and renders live block-by-block playback progress while
// audio is still playing — the view the after-the-fact Channel-1 receipt
// structurally cannot give.
//
// Discovery precedence: -f <path> flag > NARRATE_OBSERVE_FILE env > newest
// matching /tmp/narrate-observe-*.jsonl. Lifetime: runs until Ctrl-C — it
// does NOT self-exit at the end of a speak run (the scratch file is left on
// disk for the OS to reap), it keeps tailing for the next one. Foreign /
// unparseable lines are skipped silently. Stdlib only.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Mirror of cmd/narrate-mcp's wire contract. Duplicated (not imported) so the
// observer stays a standalone dumb reader with no coupling to the writer's
// composition root — the JSONL line IS the contract.
const observeSchemaName = "narrate.observe.block"

const (
	reopenDelay = 200 * time.Millisecond // retry an absent/again-absent file.
	pollDelay   = 150 * time.Millisecond // sleep on EOF before reading more.
	globPattern = "/tmp/narrate-observe-*.jsonl"
)

type blockEvent struct {
	Schema            string `json:"schema"`
	BlockID           string `json:"block_id"`
	Order             int    `json:"order"`
	Total             int    `json:"total"`
	Level             int    `json:"level"`
	Status            string `json:"status"`
	PlannedDurationMs int64  `json:"planned_duration_ms"`
	Playing           bool   `json:"playing"`
}

// renderLine parses one JSONL line into a human progress line. ok=false means
// "skip silently" — a blank line, malformed JSON, or a foreign schema (the
// observer tolerates other tools writing to the same file). Pure → unit
// tested without a tail loop.
//
// Format: "[3/9] L2 voiced 4.2s > b3". The marker is '>' when the block's
// audio is playing, '.' when it has none (pause / refused notice block).
func renderLine(raw []byte) (string, bool) {
	var ev blockEvent
	if err := json.Unmarshal(raw, &ev); err != nil || ev.Schema != observeSchemaName {
		return "", false
	}
	marker := "."
	if ev.Playing {
		marker = ">"
	}
	secs := float64(ev.PlannedDurationMs) / 1000.0
	return fmt.Sprintf("[%d/%d] L%d %s %.1fs %s %s",
		ev.Order, ev.Total, ev.Level, ev.Status, secs, marker, ev.BlockID), true
}

// resolveObserveTarget applies discovery precedence: explicit -f flag, then
// NARRATE_OBSERVE_FILE, then the newest file matching globPattern. Returns an
// error only when the glob fallback finds nothing (the flag/env paths are
// honored verbatim even if not yet created — tail waits for them to appear).
func resolveObserveTarget(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	if env := os.Getenv("NARRATE_OBSERVE_FILE"); env != "" {
		return env, nil
	}
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no scratch file found at %s; pass -f <path> or set NARRATE_OBSERVE_FILE", globPattern)
	}
	// Newest by mtime — the most recent speak call.
	sort.Slice(matches, func(i, j int) bool {
		return fileModTime(matches[i]).After(fileModTime(matches[j]))
	})
	return matches[0], nil
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// tail follows path forever, rendering each complete JSONL line to out. It
// buffers a partial trailing line until its newline arrives (the writer
// issues one Write per pre-newlined line, so a reader mid-write sees at most a
// truncated tail, never an interleaving). Returns only on a read error other
// than EOF; an absent file is retried (run-until-Ctrl-C lifetime).
func tail(path string, out io.Writer) error {
	var f *os.File
	for {
		var err error
		f, err = os.Open(path)
		if err == nil {
			break
		}
		time.Sleep(reopenDelay)
	}
	defer func() { _ = f.Close() }()

	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := f.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			buf = drainLines(buf, out)
		}
		if err == io.EOF {
			time.Sleep(pollDelay)
			continue
		}
		if err != nil {
			return err
		}
	}
}

// drainLines renders every COMPLETE (newline-terminated) line in buf and
// returns the leftover partial tail to carry into the next read.
func drainLines(buf []byte, out io.Writer) []byte {
	for {
		nl := bytes.IndexByte(buf, '\n')
		if nl < 0 {
			return buf
		}
		line := buf[:nl]
		buf = buf[nl+1:]
		if s, ok := renderLine(line); ok {
			_, _ = fmt.Fprintln(out, s)
		}
	}
}

func main() {
	flagPath := flag.String("f", "", "scratch JSONL file to tail (default: NARRATE_OBSERVE_FILE, else newest "+globPattern+")")
	flag.Parse()

	target, err := resolveObserveTarget(*flagPath)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "narrate-observe: "+err.Error())
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "narrate-observe: tailing %s (Ctrl-C to stop)\n", target)
	if err := tail(target, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "narrate-observe: "+err.Error())
		os.Exit(1)
	}
}
