package animedb

import (
	"regexp"
	"strings"
)

var (
	bracketRe    = regexp.MustCompile(`\[[^\]]*\]`)
	parenRe      = regexp.MustCompile(`\([^)]*\)`)
	seasonRe     = regexp.MustCompile(`(?i)\b(?:season\s*\d+|\d+(?:st|nd|rd|th)\s*season|part\s*\d+|cour\s*\d+)\b`)
	romanSuffixRe = regexp.MustCompile(`(?i)\s+(?:ii|iii|iv|v|vi|vii|viii|ix|x)$`)
	noiseRe      = regexp.MustCompile(`(?i)\s+(?:batch|complete|the\s+animation)$`)
	multiSpaceRe = regexp.MustCompile(`\s+`)
)

// normalizeTitle normalizes a title for comparison.
// It lowercases, strips brackets/parens, replaces separators with spaces,
// removes season/part suffixes, trailing roman numerals, and common noise words.
func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	s = bracketRe.ReplaceAllString(s, " ")
	s = parenRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = seasonRe.ReplaceAllString(s, " ")
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = romanSuffixRe.ReplaceAllString(s, "")
	s = noiseRe.ReplaceAllString(s, "")
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return s
}
