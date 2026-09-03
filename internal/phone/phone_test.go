package phone

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"+15551234567", "15551234567"},
		{"15551234567", "15551234567"},
		{"+1 (555) 123-4567", "15551234567"},
		{"  +62 812-3456-7890 ", "6281234567890"},
		{"6281234567890", "6281234567890"},
		{"(021) 123-4567", "0211234567"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := Normalize(c.input); got != c.want {
			t.Fatalf("Normalize(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestEqual(t *testing.T) {
	if !Equal("+15551234567", "15551234567") {
		t.Fatal("expected plus-prefixed and bare number to be equal")
	}
	if !Equal("+1 (555) 123-4567", "15551234567") {
		t.Fatal("expected formatted and plain to be equal")
	}
	if Equal("15551234567", "15551234568") {
		t.Fatal("expected different numbers to not be equal")
	}
}

func TestNormalizeStripsPlusPrefix(t *testing.T) {
	// Core requirement: + prefix inconsistencies must not break matching
	plus := Normalize("+6281234567890")
	bare := Normalize("6281234567890")
	if plus != bare {
		t.Fatalf("plus %q != bare %q", plus, bare)
	}
}
