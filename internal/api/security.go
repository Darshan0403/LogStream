// internal/api/security.go
package api

import (
	"crypto/sha256"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Config bundles the security-relevant knobs for the HTTP/WS layer.
type Config struct {
	APIKey    string // dashboard read/write key (X-API-Key)
	IngestKey string // key accepted on POST /ingest

	// WSJWTSecret signs and verifies the short-lived WebSocket tokens.
	// Kept separate from APIKey so the two trust domains are decoupled (H1).
	WSJWTSecret []byte

	// AllowedOrigins is the CORS / WebSocket origin allowlist (H6).
	// Empty  => browsers from other origins are refused (same-origin only).
	// ["*"]  => any origin (development convenience only).
	AllowedOrigins []string

	// TrustedProxies are CIDRs whose X-Forwarded-For header we believe (H2).
	TrustedProxies []*net.IPNet
}

// DeriveWSSecret returns the configured secret, or a deterministic key derived
// from the API key when none is set. Derivation keeps the raw API key from
// doubling as the JWT HMAC secret while avoiding an extra required env var.
func DeriveWSSecret(explicit, apiKey string) []byte {
	if explicit != "" {
		return []byte(explicit)
	}
	sum := sha256.Sum256([]byte("logstream:ws-jwt:v1:" + apiKey))
	return sum[:]
}

// ParseCIDRs turns a comma-separated list of CIDRs / bare IPs into networks.
// Unparseable entries are skipped. Bare IPs become /32 or /128.
func ParseCIDRs(csv string) []*net.IPNet {
	var out []*net.IPNet
	for _, raw := range strings.Split(csv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			if ip := net.ParseIP(raw); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				raw = raw + "/" + strconv.Itoa(bits)
			}
		}
		if _, n, err := net.ParseCIDR(raw); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// DefaultTrustedProxies covers loopback and RFC1918 ranges — the usual place a
// reverse proxy (nginx, a load balancer, the Docker bridge) sits.
func DefaultTrustedProxies() []*net.IPNet {
	return ParseCIDRs("127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16")
}

// ParseOrigins splits a comma-separated origin allowlist.
func ParseOrigins(csv string) []string {
	var out []string
	for _, raw := range strings.Split(csv, ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			out = append(out, raw)
		}
	}
	return out
}

func originAllowed(origin string, allowed []string) bool {
	if origin == "" {
		return true // non-browser client (curl, agent) — no Origin header
	}
	for _, a := range allowed {
		if a == "*" || strings.EqualFold(a, origin) {
			return true
		}
	}
	return false
}

// sameHostOrigin reports whether the Origin header points at the same host the
// request was sent to (covers the reverse-proxied same-origin case without
// needing the public URL in config).
func sameHostOrigin(origin, host string) bool {
	if origin == "" || host == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

// clientIP resolves the real client address, trusting X-Forwarded-For only when
// the immediate peer is a trusted proxy. Walks the XFF chain right-to-left and
// returns the first address that is not itself a trusted proxy (H2).
func clientIP(remoteAddr, xff string, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil || !ipInAny(peer, trusted) || xff == "" {
		return host
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		cand := strings.TrimSpace(parts[i])
		ip := net.ParseIP(cand)
		if ip == nil {
			continue
		}
		if ipInAny(ip, trusted) {
			continue // another proxy hop — keep walking left
		}
		return cand
	}
	return host
}

func ipInAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
