package plan

import "strings"

// SchemaVersion — current narration-plan schema version, semver MAJOR.MINOR.
//
// Additive-compatible within a major: consumers ignore unknown fields.
// Bumping the major signals a non-backward-compatible change and the
// honesty rule requires we surface it (we never silently break readers).
const SchemaVersion = "1.0"

// IsCompatible reports whether a plan with schema_version=other can be safely
// read by code that targets SchemaVersion. Same-major → true. Different major,
// empty, or malformed → false.
//
// Malformed inputs are deliberately treated as incompatible rather than
// permissive: consumers should refuse to interpret a plan whose version they
// cannot verify.
func IsCompatible(other string) bool {
	wantMajor, ok := majorOf(SchemaVersion)
	if !ok {
		return false
	}
	gotMajor, ok := majorOf(other)
	if !ok {
		return false
	}
	return wantMajor == gotMajor
}

// majorOf extracts the integer major component of a "MAJOR.MINOR" version
// string. Returns ok=false for empty input, missing dot, non-numeric prefix,
// or leading/trailing whitespace.
func majorOf(v string) (string, bool) {
	if v == "" {
		return "", false
	}
	// Disallow leading/trailing whitespace — strict parse keeps the schema
	// boundary honest. Callers strip externally if needed.
	if v != strings.TrimSpace(v) {
		return "", false
	}
	i := strings.IndexByte(v, '.')
	if i <= 0 || i == len(v)-1 {
		return "", false
	}
	major := v[:i]
	for _, r := range major {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	// Reject leading zero on multi-digit majors ("01.0") to avoid two strings
	// meaning the same version.
	if len(major) > 1 && major[0] == '0' {
		return "", false
	}
	// Also confirm minor is at least one digit and all numeric.
	minor := v[i+1:]
	for _, r := range minor {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return major, true
}
