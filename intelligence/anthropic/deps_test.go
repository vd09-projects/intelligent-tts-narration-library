package anthropic

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// TestInvariant_OnlyDependsOnPlanIntelligenceAndTmpl asserts that
// nothing under intelligence/anthropic imports another package from
// this module except plan/, the parent intelligence/, and the shared
// internal/intelligencetmpl/. Mirrors intelligence/mcpsampling's
// deps_test — the second concrete IntelligenceAdapter must obey the
// same composition rule. The honesty rule + plan-is-the-contract
// invariant collapse if an adapter starts reaching for planner/,
// pipeline/, or cmd/.
func TestInvariant_OnlyDependsOnPlanIntelligenceAndTmpl(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"
	const selfPackage = modulePath + "/intelligence/anthropic"
	const planPackage = modulePath + "/plan"
	const intelligencePackage = modulePath + "/intelligence"
	const intelligenceTmplPackage = modulePath + "/internal/intelligencetmpl"

	out, err := exec.Command("go", "list", "-deps", selfPackage).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps failed: %v", err)
	}

	allowed := map[string]bool{
		selfPackage:             true,
		planPackage:             true,
		intelligencePackage:     true,
		intelligenceTmplPackage: true,
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
		t.Fatalf("intelligence/anthropic may only import plan/, intelligence/, and internal/intelligencetmpl/, found %d other internal deps:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}
