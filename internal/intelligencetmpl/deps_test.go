package intelligencetmpl

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// TestInvariant_OnlyDependsOnPlanAndIntelligence asserts that the
// shared prompt-template package imports only plan/ and the parent
// intelligence/ from this module. It is the lift target for
// mcpsampling + anthropic — any drift toward adapter / planner /
// pipeline / cmd would invert the dependency direction those adapters
// rely on.
func TestInvariant_OnlyDependsOnPlanAndIntelligence(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"
	const selfPackage = modulePath + "/internal/intelligencetmpl"
	const planPackage = modulePath + "/plan"
	const intelligencePackage = modulePath + "/intelligence"

	out, err := exec.Command("go", "list", "-deps", selfPackage).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps failed: %v", err)
	}

	allowed := map[string]bool{
		selfPackage:         true,
		planPackage:         true,
		intelligencePackage: true,
	}
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
		t.Fatalf("internal/intelligencetmpl may only import plan/ and intelligence/, found %d other internal deps:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}
