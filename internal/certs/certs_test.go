package certs

import (
	"crypto/x509"
	"net"
	"os"
	"testing"
)

func TestCoveredAndConstraints(t *testing.T) {
	if !Covered("testapp.test", DefaultPermitted) || !Covered("a.b.localhost", DefaultPermitted) {
		t.Fatal("expected default suffixes to be covered")
	}
	if Covered("example.com", DefaultPermitted) || Covered("notlocalhost", DefaultPermitted) {
		t.Fatal("unexpected coverage")
	}

	dir := t.TempDir()
	ca, regen, err := EnsureCA(dir, nil)
	if err != nil || regen {
		t.Fatalf("EnsureCA: %v regen=%v", err, regen)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)

	verify := func(domain string) error {
		cp, _, err := IssueLeaf(ca, dir, domain, []net.IP{net.ParseIP("192.168.1.20")})
		if err != nil {
			return err
		}
		b, _ := os.ReadFile(cp)
		blkCert := mustParse(t, b)
		_, err = blkCert.Verify(x509.VerifyOptions{
			Roots: pool, DNSName: domain, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		return err
	}
	if err := verify("testapp.test"); err != nil {
		t.Fatalf("testapp.test should verify: %v", err)
	}
	if err := verify("localhost"); err != nil {
		t.Fatalf("localhost should verify: %v", err)
	}
	// A real domain must be rejected by the CA's name constraints.
	if err := verify("example.com"); err == nil {
		t.Fatal("example.com must NOT verify against the constrained CA")
	}

	// Extending the constraint regenerates the CA.
	ca2, regen, err := EnsureCA(dir, []string{"myapp.dev"})
	if err != nil || !regen {
		t.Fatalf("expected regeneration, err=%v regen=%v", err, regen)
	}
	if !Covered("x.myapp.dev", ca2.Permitted) || !Covered("a.test", ca2.Permitted) {
		t.Fatal("regenerated CA must keep old and new suffixes")
	}
}
