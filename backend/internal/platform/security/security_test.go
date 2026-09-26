package security

import (
	"bytes"
	"regexp"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "+7 (900) 123-45-67", want: "+79001234567"},
		{in: "89001234567", want: "+79001234567"},
		{in: "79001234567", want: "+79001234567"},
		{in: "9001234567", want: "+79001234567"},
		{in: "12345", wantErr: true},
		{in: "+1 202 555 0100 1", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := NormalizePhone(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}

				return
			}
			if err != nil || got != c.want {
				t.Fatalf("NormalizePhone(%q) = %q, %v; want %q", c.in, got, err, c.want)
			}
		})
	}
}

func TestNormalizeAccount(t *testing.T) {
	if got := NormalizeAccount(" 12-34 5.6ab "); got != "123456AB" {
		t.Fatalf("got %q", got)
	}
}

func TestHasherIsDeterministicAndKeyed(t *testing.T) {
	a := NewHasher([]byte("secret-a-secret-a-secret-a-secret"))
	b := NewHasher([]byte("secret-b-secret-b-secret-b-secret"))

	p1, err := a.Phone("8 900 123-45-67")
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := a.Phone("+79001234567")
	if !bytes.Equal(p1, p2) {
		t.Fatal("same phone in different formats must give the same hash")
	}

	p3, _ := b.Phone("+79001234567")
	if bytes.Equal(p1, p3) {
		t.Fatal("different secrets must give different hashes")
	}

	if bytes.Equal(a.Account("79001234567"), p1) {
		t.Fatal("phone and account hashes of the same digits must differ")
	}
}

func TestNewToken(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
	seen := map[string]bool{}
	for range 100 {
		tok, err := NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(tok) {
			t.Fatalf("token %q is not URL-safe or has wrong length", tok)
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok] = true
	}
}
