package fsutil

import (
	"bytes"
	"testing"
)

func TestStripBOM(t *testing.T) {
	body := []byte(`{"a":1}`)

	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		{"utf-8 bom", append([]byte{0xEF, 0xBB, 0xBF}, body...), body},
		{"utf-16 le bom", append([]byte{0xFF, 0xFE}, body...), body},
		{"utf-16 be bom", append([]byte{0xFE, 0xFF}, body...), body},
		{"no bom", body, body},
		{"empty", nil, []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripBOM(tt.in); !bytes.Equal(got, tt.want) {
				t.Errorf("StripBOM = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStripBOMLeavesInnerBytesAlone guards against stripping a legitimate
// character that merely looks like a mark in the middle of the document.
func TestStripBOMLeavesInnerBytesAlone(t *testing.T) {
	in := []byte("{\"t\":\"\uFEFFinner\"}")
	if got := StripBOM(in); !bytes.Equal(got, in) {
		t.Errorf("StripBOM modified a document with no leading mark: %q", got)
	}
}
