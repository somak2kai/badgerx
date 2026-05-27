package badgerx

import (
	"fmt"
	"strings"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

type realisticUser struct {
	ID          string
	Name        string
	Email       string
	Status      string
	Role        string
	Tags        []string
	Scores      []float64
	Address     realisticAddress
	Preferences map[string]string
	Sessions    []realisticSession
	AuditLog    []auditEntry
	Notes       string
	Bio         string
	AvatarURL   string
	Active      bool
	Verified    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastLoginAt time.Time
}

type realisticAddress struct {
	Street  string
	City    string
	State   string
	Country string
	Zip     string
}

type realisticSession struct {
	Token     string
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type auditEntry struct {
	Action    string
	Resource  string
	IP        string
	Result    string
	Timestamp time.Time
}

var userAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
}

var ipBlocks = []string{"192.168", "10.0.1", "172.16.0", "203.0.113", "198.51.100"}

var auditActions = []string{
	"login", "logout", "update_profile", "change_password",
	"view_dashboard", "export_data", "invite_member", "revoke_access",
}

var auditResources = []string{
	"/api/v1/users", "/api/v1/orders", "/api/v1/reports",
	"/api/v1/settings", "/api/v1/billing", "/api/v1/integrations",
}

func makeRealisticUser() realisticUser {
	base := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)

	// 100 varied sessions
	sessions := make([]realisticSession, 100)
	for i := range sessions {
		sessions[i] = realisticSession{
			Token: fmt.Sprintf(
				"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload-%d-iat-%d-exp-%d.signature-%04d",
				i,
				base.Add(-time.Duration(i)*time.Hour).Unix(),
				base.Add(time.Duration(720-i)*time.Hour).Unix(),
				i,
			),
			IP:        fmt.Sprintf("%s.%d", ipBlocks[i%len(ipBlocks)], (i*7+13)%254+1),
			UserAgent: userAgents[i%len(userAgents)],
			CreatedAt: base.Add(-time.Duration(i) * time.Hour),
			ExpiresAt: base.Add(time.Duration(720-i) * time.Hour),
		}
	}

	// 200 audit log entries
	audit := make([]auditEntry, 200)
	for i := range audit {
		audit[i] = auditEntry{
			Action:    auditActions[i%len(auditActions)],
			Resource:  auditResources[i%len(auditResources)],
			IP:        fmt.Sprintf("%s.%d", ipBlocks[i%len(ipBlocks)], (i*13+7)%254+1),
			Result:    map[bool]string{true: "success", false: "denied"}[i%7 != 0],
			Timestamp: base.Add(-time.Duration(i) * 15 * time.Minute),
		}
	}

	return realisticUser{
		ID:     "usr_01HQZV8M9K3N5P7R2T4X6Y",
		Name:   "Alexandra Pemberton-Hughes",
		Email:  "alex.pemberton@engineering.acme.corp",
		Status: "active",
		Role:   "admin",
		Tags:   []string{"premium", "beta-tester", "enterprise", "ml-enabled", "sso-user", "verified"},
		Scores: []float64{98.7, 92.1, 87.5, 94.3, 89.9, 96.2, 91.8, 88.4, 93.7, 90.1},
		Address: realisticAddress{
			Street:  "742 Evergreen Terrace, Suite 300",
			City:    "San Francisco",
			State:   "California",
			Country: "United States",
			Zip:     "94102",
		},
		Preferences: map[string]string{
			"theme":          "dark",
			"language":       "en-US",
			"timezone":       "America/Los_Angeles",
			"notifications":  "email,slack",
			"date_format":    "YYYY-MM-DD",
			"results_per_pg": "50",
			"sidebar":        "expanded",
			"default_view":   "list",
			"export_format":  "csv",
			"2fa_method":     "totp",
		},
		Sessions: sessions,
		AuditLog: audit,
		Notes: strings.Repeat(
			"Customer escalated issue with SSO integration. "+
				"Engineering team investigating root cause. "+
				"Temporary workaround provided. "+
				"Follow-up scheduled for next sprint. ",
			5,
		),
		Bio:         "Principal engineer at ACME Corp. Previously at Google and Stripe. Open source contributor. Specialises in distributed systems, Go, and Rust. Regular speaker at GopherCon.",
		AvatarURL:   "https://cdn.acme.corp/avatars/usr_01HQZV8M9K3N5P7R2T4X6Y/profile_v3.jpg?size=512&format=webp&v=1716825600",
		Active:      true,
		Verified:    true,
		CreatedAt:   base.Add(-365 * 24 * time.Hour),
		UpdatedAt:   base,
		LastLoginAt: base.Add(-2 * time.Hour),
	}
}

// newZstd creates a ZstdCompressor and registers cleanup on the test/benchmark.
func newZstd(t testing.TB) *ZstdCompressor {
	t.Helper()
	c, err := NewZstdCompressor()
	if err != nil {
		t.Fatalf("zstd init: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// TestCompressionRatios prints a table of encoded byte sizes and compression
// ratios for every encoder/compressor combo on a realistic struct.
//
// Run with: go test -v -run TestCompressionRatios
func TestCompressionRatios(t *testing.T) {
	user := makeRealisticUser()

	type combo struct {
		name       string
		encoder    Encoder
		compressor Compressor
	}

	combos := []combo{
		{"gob  + none  ", &GobEncoderDecoder{}, &DefaultNoOpCompressor{}},
		{"gob  + snappy", &GobEncoderDecoder{}, &SnappyCompressor{}},
		{"gob  + zstd  ", &GobEncoderDecoder{}, newZstd(t)},
		{"json + none  ", &JsonEncoderDecoder{}, &DefaultNoOpCompressor{}},
		{"json + snappy", &JsonEncoderDecoder{}, &SnappyCompressor{}},
		{"json + zstd  ", &JsonEncoderDecoder{}, newZstd(t)},
	}

	gobRaw, _ := combos[0].encoder.Encode(user)
	jsonRaw, _ := combos[3].encoder.Encode(user)

	t.Logf("\n%-20s  %8s  %10s  %10s", "combo", "bytes", "vs gob", "vs json")
	t.Logf("%s", strings.Repeat("-", 56))

	for _, c := range combos {
		encoded, err := c.encoder.Encode(user)
		if err != nil {
			t.Fatalf("%s encode: %v", c.name, err)
		}
		compressed, err := c.compressor.Compress(encoded)
		if err != nil {
			t.Fatalf("%s compress: %v", c.name, err)
		}
		final := encoded
		if len(compressed) > 0 {
			final = compressed
		}
		vsGob := float64(len(gobRaw)) / float64(len(final))
		vsJson := float64(len(jsonRaw)) / float64(len(final))
		t.Logf("%-20s  %8d  %9.2fx  %9.2fx", c.name, len(final), vsGob, vsJson)
	}

	t.Logf("\nraw gob bytes:  %d", len(gobRaw))
	t.Logf("raw json bytes: %d", len(jsonRaw))
}

// benchOpts returns fresh BdOptions for each run — critical for zstd, which
// must not be shared across sub-benchmark invocations (the framework calls the
// function multiple times with increasing b.N; sharing one compressor causes
// "decoder used after Close" on the second invocation).
func benchOpts(name string) []BdOptions {
	switch name {
	case "gob+snappy":
		return []BdOptions{WithCompressor(&SnappyCompressor{})}
	case "gob+zstd":
		c, _ := NewZstdCompressor()
		return []BdOptions{WithCompressor(c)}
	case "json+none":
		return []BdOptions{WithEncoder(&JsonEncoderDecoder{})}
	case "json+snappy":
		return []BdOptions{WithEncoder(&JsonEncoderDecoder{}), WithCompressor(&SnappyCompressor{})}
	case "json+zstd":
		c, _ := NewZstdCompressor()
		return []BdOptions{WithEncoder(&JsonEncoderDecoder{}), WithCompressor(c)}
	default: // gob+none
		return nil
	}
}

var benchNames = []string{
	"gob+none", "gob+snappy", "gob+zstd",
	"json+none", "json+snappy", "json+zstd",
}

// BenchmarkRealWorld_Update measures write throughput on a realistic struct.
// Run with: go test -bench=BenchmarkRealWorld -benchmem -benchtime=5s
func BenchmarkRealWorld_Update(b *testing.B) {
	user := makeRealisticUser()
	for _, name := range benchNames {
		b.Run(name, func(b *testing.B) {
			// opts created INSIDE b.Run so each framework invocation gets a
			// fresh compressor — avoids "decoder used after Close" on zstd.
			opts := benchOpts(name)
			bdb, err := badger.Open(badger.DefaultOptions(b.TempDir()).WithLogger(nil))
			if err != nil {
				b.Fatal(err)
			}
			xdb := NewBadgerXDb(bdb, opts...)
			b.Cleanup(func() { xdb.Close() })

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := []byte(fmt.Sprintf("user:%d", i))
				if err := xdb.Update(key, user); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRealWorld_View measures read throughput on a realistic struct.
// Run with: go test -bench=BenchmarkRealWorld -benchmem -benchtime=5s
func BenchmarkRealWorld_View(b *testing.B) {
	user := makeRealisticUser()
	for _, name := range benchNames {
		b.Run(name, func(b *testing.B) {
			opts := benchOpts(name)
			bdb, err := badger.Open(badger.DefaultOptions(b.TempDir()).WithLogger(nil))
			if err != nil {
				b.Fatal(err)
			}
			xdb := NewBadgerXDb(bdb, opts...)
			b.Cleanup(func() { xdb.Close() })

			keys := make([][]byte, b.N)
			for i := 0; i < b.N; i++ {
				keys[i] = []byte(fmt.Sprintf("user:%d", i))
				if err := xdb.Update(keys[i], user); err != nil {
					b.Fatalf("pre-populate: %v", err)
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var got realisticUser
				if err := xdb.View(keys[i], &got); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
