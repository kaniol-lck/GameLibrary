package scraper

import (
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxDescriptionRunes bounds scraped descriptions. The limit is in runes, not
// bytes: slicing a UTF-8 string by byte count splits a multi-byte character and
// produces invalid UTF-8, which the previous implementation did to every
// Japanese and Chinese description.
const MaxDescriptionRunes = 500

// Unescape decodes HTML entities.
func Unescape(s string) string { return html.UnescapeString(s) }

// StripHTML removes tags and collapses whitespace, leaving readable text.
func StripHTML(s string) string {
	if !strings.ContainsRune(s, '<') {
		return CleanText(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return CleanText(b.String())
}

// CleanText collapses runs of whitespace and trims the result.
func CleanText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Truncate cuts s to at most max runes, appending an ellipsis when it had to.
// The result is always valid UTF-8.
func Truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	count := 0
	for i := range s {
		if count == max {
			return strings.TrimRight(s[:i], " \t\n\r") + "..."
		}
		count++
	}
	return s
}

// PrepareDescription normalises a scraped description and bounds its length.
func PrepareDescription(s string) string {
	s = CleanText(Unescape(StripHTML(s)))
	return Truncate(s, MaxDescriptionRunes)
}

// FirstOrEmpty returns the first element of a slice, or the zero value.
func FirstOrEmpty[T any](s []T) T {
	var zero T
	if len(s) == 0 {
		return zero
	}
	return s[0]
}

// splitCamelCase inserts spaces at lower-to-upper boundaries so that a folder
// name such as "LocalRPG" can be searched as "Local RPG".
func splitCamelCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
