package phone

import "strings"

// Normalize returns a canonical digit-only representation of a phone number.
// It strips all non-digit characters (including '+', spaces, dashes, parentheses)
// so that "+1 (555) 123-4567" and "15551234567" normalize to the same value.
// Empty input returns "".
func Normalize(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(phone))
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Equal reports whether two phone numbers match after normalization.
func Equal(a, b string) bool {
	return Normalize(a) == Normalize(b)
}
