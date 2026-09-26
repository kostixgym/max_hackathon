// Package security holds keyed hashing of personal identifiers and random tokens.
//
// Phone numbers and personal account numbers are never stored in plain form and
// never as a plain hash: the space of Russian phone numbers is ~10^10 and a plain
// SHA-256 is brute-forced in minutes. HMAC-SHA256 with a server secret stays
// deterministic (lookups and indexes work) but cannot be reversed without the key.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
)

// ErrInvalidPhone is returned for strings that are not a Russian phone number.
var ErrInvalidPhone = errors.New("invalid phone number")

// Hasher computes keyed hashes of normalized identifiers.
type Hasher struct {
	secret []byte
}

// NewHasher creates a hasher. The secret must be kept outside of the repository.
func NewHasher(secret []byte) *Hasher {
	return &Hasher{secret: append([]byte(nil), secret...)}
}

// Phone returns HMAC of a normalized phone number.
func (h *Hasher) Phone(raw string) ([]byte, error) {
	p, err := NormalizePhone(raw)
	if err != nil {
		return nil, err
	}

	return h.sum("phone:" + p), nil
}

// Account returns HMAC of a normalized personal account number (лицевой счёт).
func (h *Hasher) Account(raw string) []byte {
	return h.sum("account:" + NormalizeAccount(raw))
}

// sum prefixes the value with its kind so that equal strings of different kinds
// never produce equal hashes.
func (h *Hasher) sum(v string) []byte {
	m := hmac.New(sha256.New, h.secret)
	m.Write([]byte(v))

	return m.Sum(nil)
}

// NormalizePhone converts a Russian phone number to +7XXXXXXXXXX.
// Accepted forms: +7..., 7..., 8... (11 digits) and 10 digits without the country code.
func NormalizePhone(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()

	switch {
	case len(d) == 11 && (d[0] == '7' || d[0] == '8'):
		d = d[1:]
	case len(d) == 10:
	default:
		return "", ErrInvalidPhone
	}

	return "+7" + d, nil
}

// NormalizeAccount removes spaces and separators and upper-cases the account number.
func NormalizeAccount(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}

	return b.String()
}

// NewToken returns a URL-safe random token with 128 bits of entropy.
// Used for invite slugs and QR tokens: UUIDs are not access tokens (RFC 9562)
// and UUIDv7 additionally reveals the creation time.
// The result contains only [A-Za-z0-9_-], which MAX allows in startapp payloads.
func NewToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
