// Package normalize provides deterministic normalization for farm archive
// fields, so that equivalent spellings ("寿光市" / "寿光" / "ＳＨＯＵＧＵＡＮＧ")
// compare equal when detecting duplicates or merging legacy data.
package normalize

import (
	"strings"
	"unicode"
)

// foldRune maps full-width ASCII variants (U+FF01–U+FF5E) and the
// ideographic space (U+3000) back to their half-width forms.
func foldRune(r rune) rune {
	if r == '\u3000' { // ideographic space
		return ' '
	}
	if r >= '\uFF01' && r <= '\uFF5E' { // full-width ASCII variants
		return r - 0xFEE0
	}
	return r
}

// base folds full-width characters, strips all whitespace and uppercases
// ASCII letters. It is the shared core of every field normalizer.
func base(s string) string {
	mapped := strings.Map(foldRune, s)
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, mapped)
	return strings.ToUpper(compact)
}

// Name normalizes a cooperative name for duplicate detection:
// whitespace removed, full-width folded to half-width, letters uppercased.
func Name(s string) string {
	return base(s)
}

// regionSuffixes are trailing administrative suffixes dropped so that
// "寿光", "寿光市" and "寿光县" normalize to the same key. Longer suffixes
// must come first; at most one suffix is stripped.
var regionSuffixes = []string{"自治县", "市", "县", "区", "旗"}

// Region normalizes a region code/name for duplicate detection: base
// normalization plus stripping one trailing administrative suffix.
func Region(s string) string {
	n := base(s)
	for _, suf := range regionSuffixes {
		if trimmed := strings.TrimSuffix(n, suf); trimmed != "" && trimmed != n {
			return trimmed
		}
	}
	return n
}

// CertNo normalizes a qualification/certificate number for duplicate
// detection: whitespace removed, full-width folded, letters uppercased.
func CertNo(s string) string {
	return base(s)
}
