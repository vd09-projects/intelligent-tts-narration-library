//go:build manual

// Manual smoke test for the cmd/narrate vertical slice. Decision (v5) —
// convention: accepted. Build tag chosen over env var so the test is
// invisible to `go test ./...` (which is what CI / default dev runs
// invoke) and explicit when the developer asks for it via
// `go test -tags manual ./pipeline/...`.
//
// Builds the cmd/narrate binary into a temp dir, runs it against
// docs/samples/sample.md, and asserts exit 0 + a parseable SinkReceipt
// summary on stdout. Audio plays through the speakers — the listener
// confirms by ear that the bare-image block is refused honestly.
//
// This test must be run from a working directory where docs/samples/
// and scripts/kokoro both resolve relatively — i.e. the repo root. The
// runner cd's there explicitly to make the invariant testable from any
// IDE invocation.
package pipeline_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const smokeWallBudget = 30 * time.Second // generous; perf target 10 s, but cold start + build is slack

// repoRoot walks up from the current file until it finds go.mod, so the
// test works whether `go test` is invoked from the repo root or a package
// dir. Returns absolute path.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod walking up from %s", wd)
		}
		dir = parent
	}
}

func TestSmoke_NarrateRealBinary(t *testing.T) {
	root := repoRoot(t)
	samplePath := filepath.Join(root, "docs", "samples", "sample.md")
	if _, err := os.Stat(samplePath); err != nil {
		t.Fatalf("docs/samples/sample.md missing: %v", err)
	}

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "narrate")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/narrate")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/narrate: %v\n%s", err, out)
	}

	cmd := exec.Command(binPath, "--file", samplePath)
	cmd.Dir = root // cwd-relative ./scripts/kokoro lookup requires repo root.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start narrate: %v", err)
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("narrate exited non-zero: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
	case <-time.After(smokeWallBudget):
		_ = cmd.Process.Kill()
		t.Fatalf("narrate exceeded wall budget %s\nstdout so far:\n%s\nstderr so far:\n%s",
			smokeWallBudget, stdout.String(), stderr.String())
	}

	elapsed := time.Since(start)
	t.Logf("narrate wall time: %s", elapsed)

	out := stdout.String()
	if !strings.Contains(out, "blocks_played=") {
		t.Fatalf("stdout missing blocks_played key:\n%s", out)
	}

	played := parseBlocksPlayed(t, out)
	if played <= 0 {
		t.Errorf("blocks_played should be > 0; got %d in:\n%s", played, out)
	}
}

func parseBlocksPlayed(t *testing.T, line string) int {
	t.Helper()
	re := regexp.MustCompile(`blocks_played=(\d+)`)
	m := re.FindStringSubmatch(line)
	if len(m) != 2 {
		t.Fatalf("could not parse blocks_played from %q", line)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("atoi blocks_played: %v", err)
	}
	return n
}
