package api

import (
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestClientIP(t *testing.T) {
	trusted := ParseCIDRs("10.0.0.0/8,172.16.0.0/12,127.0.0.0/8")

	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{"direct, no xff", "203.0.113.7:5555", "", "203.0.113.7"},
		{"untrusted peer ignores xff", "203.0.113.7:5555", "1.2.3.4", "203.0.113.7"},
		{"trusted proxy uses xff", "172.18.0.5:40000", "198.51.100.9", "198.51.100.9"},
		{"trusted proxy, spoofed prefix", "172.18.0.5:40000", "9.9.9.9, 198.51.100.9", "198.51.100.9"},
		{"two proxy hops", "10.0.0.1:1", "198.51.100.9, 10.1.2.3", "198.51.100.9"},
		{"xff all trusted falls back to peer host", "10.0.0.1:1", "10.9.9.9, 10.1.2.3", "10.0.0.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clientIP(c.remoteAddr, c.xff, trusted); got != c.want {
				t.Fatalf("clientIP(%q, %q) = %q, want %q", c.remoteAddr, c.xff, got, c.want)
			}
		})
	}
}

func TestOriginAllowed(t *testing.T) {
	allow := []string{"http://localhost:5173", "https://logs.example.com"}
	if !originAllowed("", allow) {
		t.Fatal("empty origin (non-browser) should be allowed")
	}
	if !originAllowed("https://logs.example.com", allow) {
		t.Fatal("listed origin should be allowed")
	}
	if originAllowed("https://evil.example", allow) {
		t.Fatal("unlisted origin must be rejected")
	}
	if !originAllowed("anything", []string{"*"}) {
		t.Fatal("wildcard should allow any origin")
	}
}

func TestSameHostOrigin(t *testing.T) {
	if !sameHostOrigin("http://localhost:5173", "localhost:5173") {
		t.Fatal("same host should match")
	}
	if sameHostOrigin("http://localhost:5173", "api.other:8090") {
		t.Fatal("different host must not match")
	}
}

func TestDeriveWSSecret(t *testing.T) {
	explicit := DeriveWSSecret("my-explicit-secret", "api-key")
	if string(explicit) != "my-explicit-secret" {
		t.Fatal("explicit secret should be used verbatim")
	}
	d1 := DeriveWSSecret("", "api-key-1")
	d2 := DeriveWSSecret("", "api-key-1")
	d3 := DeriveWSSecret("", "api-key-2")
	if string(d1) != string(d2) {
		t.Fatal("derivation must be deterministic")
	}
	if string(d1) == string(d3) {
		t.Fatal("different API keys must derive different secrets")
	}
	if string(d1) == "api-key-1" {
		t.Fatal("derived secret must not equal the raw API key")
	}
}

// TestWSTokenRoundTrip proves the HS256 pinning + expiry requirement (H1).
func TestWSTokenValidation(t *testing.T) {
	secret := DeriveWSSecret("", "some-api-key")

	parse := func(tok string) error {
		parsed, err := jwt.Parse(tok,
			func(*jwt.Token) (interface{}, error) { return secret, nil },
			jwt.WithValidMethods([]string{"HS256"}),
			jwt.WithExpirationRequired(),
		)
		if err != nil {
			return err
		}
		if !parsed.Valid {
			return jwt.ErrTokenInvalidClaims
		}
		return nil
	}

	good := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	})
	goodStr, _ := good.SignedString(secret)
	if err := parse(goodStr); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}

	noExp := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{})
	noExpStr, _ := noExp.SignedString(secret)
	if err := parse(noExpStr); err == nil {
		t.Fatal("token without exp must be rejected")
	}

	expired := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	})
	expiredStr, _ := expired.SignedString(secret)
	if err := parse(expiredStr); err == nil {
		t.Fatal("expired token must be rejected")
	}

	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	})
	noneStr, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err := parse(noneStr); err == nil {
		t.Fatal("alg=none token must be rejected")
	}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestParseCIDRs(t *testing.T) {
	got := ParseCIDRs("10.0.0.0/8, 192.168.1.5 , garbage, ::1/128")
	if len(got) != 3 {
		t.Fatalf("expected 3 nets, got %d (%v)", len(got), got)
	}
	if !got[1].Contains(net.ParseIP("192.168.1.5")) {
		t.Fatal("bare IP should become a /32 containing itself")
	}
	_ = mustCIDR(t, "10.0.0.0/8")
}
