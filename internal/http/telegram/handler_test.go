package telegram

import "testing"

func TestExtractSearchQuery(t *testing.T) {
	cases := []struct {
		text  string
		query string
		found bool
	}{
		// Questions mentioning "cari file" must go to the AI chat, not search.
		{text: "kalau aku mau cari file bagaimana caranya ?", query: "", found: false},
		{text: "bagaimana cara cari gambar di bot ini?", query: "", found: false},
		{text: "tolong cari file laporan", query: "", found: false},
		{text: "halo", query: "", found: false},
		// Explicit search commands.
		{text: "cari file laporan", query: "laporan", found: true},
		{text: "cari gambar kucing", query: "kucing", found: true},
		{text: "cari file atau gambar kucing", query: "kucing", found: true},
		{text: "cari file atau gambar dengan nama invoice 2024", query: "invoice 2024", found: true},
		{text: "cari laporan", query: "laporan", found: true},
		{text: "find my report", query: "report", found: true},
		{text: "find my report file", query: "report", found: true},
		// Words ending in "file" must not be truncated.
		{text: "cari profile", query: "profile", found: true},
		// Bare command asks for usage.
		{text: "cari file", query: "", found: true},
	}
	for _, tc := range cases {
		q, ok := extractSearchQuery(tc.text)
		if ok != tc.found || q != tc.query {
			t.Errorf("extractSearchQuery(%q) = (%q, %v), want (%q, %v)", tc.text, q, ok, tc.query, tc.found)
		}
	}
}
