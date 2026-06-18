package adapter

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// TestInvariant_AdapterImportsOnlyPlan asserts that adapter/ and
// adapter/file (the only concrete adapter shipped phase one) import
// no internal project packages other than plan/.
//
// Mirrors plan/deps_test.go: shells `go list -deps <pkg>` and scans
// for module-path prefixes that are not the package itself or plan.
// Skipped when `go` is not on PATH.
//
// When more concrete adapters land (mcptext, ocr), add their import
// paths to packagesToCheck.
func TestInvariant_AdapterImportsOnlyPlan(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping adapter deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"
	const planPackage = modulePath + "/plan"

	packagesToCheck := []string{
		modulePath + "/adapter",
		modulePath + "/adapter/file",
	}

	allowedInternal := map[string]bool{
		planPackage: true,
	}
	for _, p := range packagesToCheck {
		allowedInternal[p] = true
	}

	for _, pkg := range packagesToCheck {
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()
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
				t.Fatalf("%s must import only plan/ + stdlib internally, found %d disallowed deps:\n  %s",
					pkg, len(violations), strings.Join(violations, "\n  "))
			}
		})
	}
}
