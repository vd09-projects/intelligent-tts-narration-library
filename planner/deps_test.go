package planner

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// TestInvariant_NoDirectIOImports asserts that planner package SOURCE
// FILES (not transitive deps) import no I/O stdlib package and no
// internal package other than plan/, adapter/, and intelligence/
// (interface only).
//
// Why direct imports, not transitive: goldmark — our chosen markdown
// parser — pulls in net/url and a few syscall-using stdlib packages
// transitively. The CLAUDE.md invariant is about preventing planner
// source from REACHING for I/O, not preventing the segmenter library
// from doing whatever its own house-keeping needs. Goldmark itself is
// explicitly sanctioned by the design doc as the segmenter.
//
// Mirrors plan/deps_test.go, adapter/deps_test.go,
// intelligence/deps_test.go but inverts the check from -deps to the
// package's own Imports field.
func TestInvariant_NoDirectIOImports(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"
	const selfPackage = modulePath + "/planner"

	// {{.Imports}} prints the imports of the package itself (test
	// files excluded). That is exactly the set the CLAUDE.md invariant
	// constrains.
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", selfPackage).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list failed: %v", err)
	}

	// Stdlib I/O packages the planner source must not import directly.
	stdlibForbidden := map[string]bool{
		"os":        true,
		"net":       true,
		"net/http":  true,
		"net/url":   true,
		"net/rpc":   true,
		"io/ioutil": true,
		"syscall":   true,
		"os/exec":   true,
		"os/signal": true,
		"os/user":   true,
	}

	// Internal packages the planner must not import. Only plan/,
	// adapter/ (interface), and intelligence/ (interface) are allowed.
	forbiddenInternalPrefixes := []string{
		modulePath + "/adapter/file",
		modulePath + "/adapter/mcptext",
		modulePath + "/adapter/ocr",
		modulePath + "/render/",
		modulePath + "/sink/",
		modulePath + "/intelligence/mcpsampling",
		modulePath + "/intelligence/anthropic",
		modulePath + "/pipeline",
		modulePath + "/cmd/",
	}

	var violations []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		imp := strings.TrimSpace(scanner.Text())
		if imp == "" {
			continue
		}
		if stdlibForbidden[imp] {
			violations = append(violations, imp)
			continue
		}
		for _, p := range forbiddenInternalPrefixes {
			if imp == strings.TrimSuffix(p, "/") || strings.HasPrefix(imp, p) {
				violations = append(violations, imp)
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go list output: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("planner/ source files may not import any of these (%d violations):\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}
