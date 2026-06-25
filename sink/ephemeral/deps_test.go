package ephemeral

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// TestInvariant_EphemeralImportsOnlyEdgePackages asserts that sink/ephemeral
// imports no internal project package other than plan/ + render/ + sink/
// (plus itself). This is the import-wall guard for the issue #81 observer
// seam: the per-block emit lives here, but the concrete Channel-2 writer
// (JSONL, scratch-file lifecycle) stays at the composition root, so the sink
// must NOT pick up a dependency on cmd/, pipeline/, planner/, adapter/, or
// intelligence/.
//
// Mirrors adapter/deps_test.go: shells `go list -deps <pkg>` and scans for
// module-path prefixes outside the allow-list. Skipped when `go` is absent.
func TestInvariant_EphemeralImportsOnlyEdgePackages(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping ephemeral deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"

	pkg := modulePath + "/sink/ephemeral"
	allowedInternal := map[string]bool{
		modulePath + "/plan":           true,
		modulePath + "/render":         true,
		modulePath + "/sink":           true,
		modulePath + "/sink/ephemeral": true,
	}

	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps %s failed: %v\nstderr:\n%s", pkg, err, ee.Stderr)
		}
		t.Fatalf("go list -deps %s failed: %v", pkg, err)
	}

	var violations []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		dep := strings.TrimSpace(scanner.Text())
		if dep == "" || !strings.HasPrefix(dep, modulePath) {
			continue
		}
		if allowedInternal[dep] {
			continue
		}
		violations = append(violations, dep)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go list output: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("%s must import only plan/ + render/ + sink/ + stdlib internally, found %d disallowed deps:\n  %s",
			pkg, len(violations), strings.Join(violations, "\n  "))
	}
}
