package intelligence

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// TestInvariant_OnlyDependsOnPlan asserts that nothing under
// ./intelligence/... imports another package from this module except
// plan/. This package is the pluggable-comprehension contract; importing
// a concrete adapter, the planner, or any edge package would invert the
// dependency direction.
//
// Mirrors plan/deps_test.go and adapter/deps_test.go. Shells `go list
// -deps` and scans for module-qualified deps; allowlist = {self, plan/}.
func TestInvariant_OnlyDependsOnPlan(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"
	const selfPackage = modulePath + "/intelligence"
	const planPackage = modulePath + "/plan"

	out, err := exec.Command("go", "list", "-deps", selfPackage).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps failed: %v", err)
	}

	allowed := map[string]bool{selfPackage: true, planPackage: true}
	var violations []string

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		dep := strings.TrimSpace(scanner.Text())
		if dep == "" || !strings.HasPrefix(dep, modulePath) {
			continue
		}
		if allowed[dep] {
			continue
		}
		violations = append(violations, dep)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go list output: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("intelligence/ may only import plan/, found %d other internal deps:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}
