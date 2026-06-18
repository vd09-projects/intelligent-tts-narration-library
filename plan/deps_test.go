package plan

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// Decision (3) — tradeoff: use a `go list -deps` subprocess to enforce the
// zero-deps invariant instead of an in-process go/build AST traversal.
// Simpler, mirrors the human invariant check, and any drift produces output
// the developer can copy-paste-investigate. Status: accepted.

// TestInvariant_ZeroInternalDeps asserts that nothing under ./plan/...
// imports another package from this module. plan/ is the schema contract;
// any internal import would invert the dependency direction.
//
// The test shells `go list -deps ./plan/...` and scans for lines starting
// with the module path. Skipped when `go` is not on PATH (e.g. minimal CI
// containers); on developer machines and standard CI it always runs.
func TestInvariant_ZeroInternalDeps(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping zero-deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"
	const selfPackage = modulePath + "/plan"

	// Scope the check to the plan package by its module-qualified import path
	// — matches the AC (CLAUDE.md) and the test name. We cannot use the
	// "./plan/..." relative pattern because `go test` runs each package's
	// tests with the package dir as cwd, so "./plan/..." would resolve to
	// "plan/plan", which doesn't exist. Using the module-qualified path makes
	// the test correct regardless of cwd.
	out, err := exec.Command("go", "list", "-deps", selfPackage).Output()
	if err != nil {
		// Surface stderr for easier diagnosis (e.g. unrelated build failure).
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps failed: %v", err)
	}

	var violations []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		dep := strings.TrimSpace(scanner.Text())
		if dep == "" {
			continue
		}
		if !strings.HasPrefix(dep, modulePath) {
			continue
		}
		if dep == selfPackage {
			continue
		}
		violations = append(violations, dep)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go list output: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("plan/ must have zero internal-project dependencies, found %d:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}
