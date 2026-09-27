package registry

import (
	"strings"
	"unicode/utf8"
)

// MaskName shortens the full name of a person to «Фамилия И. О.»: registry names are
// personal data, so the lists other people see (owner directories, the meeting
// tracker) show only the masked form. Organizations and municipalities are not
// persons and keep their names.
func MaskName(fullName, kind string) string {
	if kind != "person" {
		return fullName
	}
	parts := strings.Fields(fullName)
	if len(parts) < 2 {
		return fullName
	}
	masked := []string{parts[0]}
	for _, part := range parts[1:] {
		r, _ := utf8.DecodeRuneInString(part)
		if r != utf8.RuneError {
			masked = append(masked, string(r)+".")
		}
	}

	return strings.Join(masked, " ")
}
