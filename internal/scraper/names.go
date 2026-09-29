package scraper

import (
	"regexp"
	"strings"
)

// rjPattern matches a DLsite product code such as RJ123456.
//
// The code is written in either case in the wild ("RJ123456" on the store page,
// "rj123456" in somebody's folder name) and is matched case-insensitively, and it
// is anchored on word boundaries so that a folder named "XRJ1234567" does not
// masquerade as a product code.
var rjPattern = regexp.MustCompile(`\b(?i:RJ)(\d{6}|\d{8})\b`)

// RJCode extracts a normalised (upper-case) DLsite product code from a string.
func RJCode(s string) string {
	match := rjPattern.FindString(s)
	if match == "" {
		return ""
	}
	return strings.ToUpper(match)
}

// BaseName returns the folder name a game directory is known by.
func BaseName(gameDir string) string {
	cleaned := strings.TrimRight(strings.ReplaceAll(gameDir, `\`, "/"), "/")
	if idx := strings.LastIndexByte(cleaned, '/'); idx >= 0 {
		cleaned = cleaned[idx+1:]
	}
	return strings.TrimSpace(cleaned)
}

// SearchName turns a folder name into a plausible search term.
//
// Steam folders created by manual installs are frequently named
// "steam_<appid>"; the numeric part is the useful search key and the prefix is
// noise. Beyond that the name is used as-is: the caller feeds it through
// SearchTerms, which generates the variations.
func SearchName(folderName string) string {
	name := strings.TrimSpace(folderName)
	if name == "" {
		return ""
	}
	if rest, ok := cutPrefixFold(name, "steam_"); ok {
		if isDigits(rest) {
			return rest
		}
	}
	return name
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return s, false
	}
	return s[len(prefix):], true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SearchTerms generates the name variants worth trying against a metadata
// source, in order of fidelity: the original, camel case split, separators
// turned into spaces, and finally a punctuation-free form.
//
// Variants that duplicate an earlier one are dropped, so the caller never spends
// a rate-limited request on a repeat.
func SearchTerms(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	candidates := []string{
		name,
		splitCamelCase(name),
		replaceSeparators(name),
		replaceSeparators(splitCamelCase(name)),
		strings.Join(strings.Fields(stripEditionSuffixes(name)), " "),
	}

	seen := make(map[string]bool, len(candidates))
	terms := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = CleanText(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		terms = append(terms, candidate)
	}
	return terms
}

func replaceSeparators(s string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ", ".", " ", "+", " ")
	return CleanText(replacer.Replace(s))
}

// editionSuffixes are trailing markers that hurt search precision. Release group
// tags and repack markers are extremely common in folder names.
var editionSuffixes = []string{
	"repack", "proper", "cracked", "crack", "multi", "rip", "portable",
	"goty", "definitive edition", "complete edition", "deluxe edition",
	"v1", "v2", "fitgirl", "codex", "plaza", "skidrow", "razor1911",
}

// stripEditionSuffixes removes trailing release markers and version tags.
func stripEditionSuffixes(name string) string {
	out := name
	for changed := true; changed; {
		changed = false
		trimmed := strings.TrimSpace(out)
		// Trailing bracketed groups: "Game (2019)" / "Game [FitGirl Repack]".
		if strings.HasSuffix(trimmed, ")") {
			if idx := strings.LastIndexByte(trimmed, '('); idx > 0 {
				if inner := trimmed[idx+1 : len(trimmed)-1]; looksLikeMetadata(inner) {
					out, changed = strings.TrimSpace(trimmed[:idx]), true
					continue
				}
			}
		}
		if strings.HasSuffix(trimmed, "]") {
			if idx := strings.LastIndexByte(trimmed, '['); idx > 0 {
				out, changed = strings.TrimSpace(trimmed[:idx]), true
				continue
			}
		}
		lowered := strings.ToLower(trimmed)
		for _, suffix := range editionSuffixes {
			if strings.HasSuffix(lowered, " "+suffix) {
				out, changed = strings.TrimSpace(trimmed[:len(trimmed)-len(suffix)]), true
				break
			}
		}
	}
	return out
}

func looksLikeMetadata(inner string) bool {
	inner = strings.ToLower(strings.TrimSpace(inner))
	if inner == "" {
		return false
	}
	if isDigits(inner) {
		return true
	}
	for _, suffix := range editionSuffixes {
		if strings.Contains(inner, suffix) {
			return true
		}
	}
	return strings.HasPrefix(inner, "repack") || strings.Contains(inner, "bit")
}
