// Package certs creates a name-constrained local CA and leaf certificates.
//
// The CA may only sign names under the permitted DNS domains (default:
// localhost and test) and loopback/private IP ranges. Even if its key leaked,
// it could not be used to impersonate real sites.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const CAName = "Laraserve Local CA"

// DefaultPermitted are the DNS suffixes the CA is allowed to sign by default.
var DefaultPermitted = []string{"localhost", "test"}

var privateRanges = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7",
}

type CA struct {
	Cert      *x509.Certificate
	Key       *ecdsa.PrivateKey
	PEMPath   string
	SHA256    string
	SHA1      string
	Permitted []string
}

// Covered reports whether domain falls under one of the permitted suffixes.
func Covered(domain string, permitted []string) bool {
	domain = strings.ToLower(domain)
	for _, p := range permitted {
		p = strings.ToLower(p)
		if domain == p || strings.HasSuffix(domain, "."+p) {
			return true
		}
	}
	return false
}

// EnsureCA loads the CA in dir, or creates it. If the stored CA does not
// permit every suffix in `permitted`, a new CA is generated (regenerated=true).
func EnsureCA(dir string, permitted []string) (ca *CA, regenerated bool, err error) {
	permitted = normalize(permitted)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	certPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca.key")
	listPath := filepath.Join(dir, "permitted.json")

	if existing, err := loadCA(certPath, keyPath); err == nil {
		var have []string
		if b, err := os.ReadFile(listPath); err == nil {
			_ = json.Unmarshal(b, &have)
		}
		if subset(permitted, have) && time.Now().Before(existing.Cert.NotAfter.AddDate(0, 0, -30)) {
			existing.Permitted = have
			return existing, false, nil
		}
		permitted = normalize(append(permitted, have...))
		regenerated = true
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, false, err
	}
	var ranges []*net.IPNet
	for _, r := range privateRanges {
		_, n, _ := net.ParseCIDR(r)
		ranges = append(ranges, n)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:                newSerial(),
		Subject:                     pkix.Name{CommonName: CAName, Organization: []string{"Laraserve"}},
		NotBefore:                   now.Add(-time.Hour),
		NotAfter:                    now.AddDate(10, 0, 0),
		IsCA:                        true,
		BasicConstraintsValid:       true,
		MaxPathLen:                  0,
		MaxPathLenZero:              true,
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         permitted,
		PermittedIPRanges:           ranges,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, false, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, false, err
	}
	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return nil, false, err
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return nil, false, err
	}
	lb, _ := json.Marshal(permitted)
	if err := os.WriteFile(listPath, lb, 0o600); err != nil {
		return nil, false, err
	}
	_ = os.RemoveAll(filepath.Join(dir, "leaf")) // old leaves are signed by the old CA
	ca, err = loadCA(certPath, keyPath)
	if err != nil {
		return nil, false, err
	}
	ca.Permitted = permitted
	return ca, regenerated, nil
}

// IssueLeaf returns paths to a certificate valid for domain, localhost,
// loopback and any extra IPs. An existing, still valid leaf is reused.
func IssueLeaf(ca *CA, dir, domain string, extraIPs []net.IP) (certPath, keyPath string, err error) {
	leafDir := filepath.Join(dir, "leaf")
	if err := os.MkdirAll(leafDir, 0o700); err != nil {
		return "", "", err
	}
	safe := strings.NewReplacer("*", "_", "/", "_", "\\", "_").Replace(domain)
	certPath = filepath.Join(leafDir, safe+".pem")
	keyPath = filepath.Join(leafDir, safe+".key")

	names := uniq([]string{domain, "localhost"})
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	ips = append(ips, extraIPs...)

	if leafOK(certPath, keyPath, ca, names, ips) {
		return certPath, keyPath, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          newSerial(),
		Subject:               pkix.Name{CommonName: domain, Organization: []string{"Laraserve"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(0, 0, 365),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              names,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return "", "", err
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

// LANIPs returns the machine's private (RFC 1918) IPv4 addresses.
func LANIPs() []net.IP {
	var out []net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
			out = append(out, n.IP)
		}
	}
	return out
}

func leafOK(certPath, keyPath string, ca *CA, names []string, ips []net.IP) bool {
	b, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	if _, err := os.Stat(keyPath); err != nil {
		return false
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return false
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil || time.Now().After(c.NotAfter.AddDate(0, 0, -30)) {
		return false
	}
	if c.CheckSignatureFrom(ca.Cert) != nil {
		return false
	}
	have := map[string]bool{}
	for _, n := range c.DNSNames {
		have[n] = true
	}
	for _, ip := range c.IPAddresses {
		have[ip.String()] = true
	}
	for _, n := range names {
		if !have[n] {
			return false
		}
	}
	for _, ip := range ips {
		if !have[ip.String()] {
			return false
		}
	}
	return true
}

func loadCA(certPath, keyPath string) (*CA, error) {
	cb, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	kb, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	cblk, _ := pem.Decode(cb)
	kblk, _ := pem.Decode(kb)
	if cblk == nil || kblk == nil {
		return nil, errors.New("invalid CA PEM")
	}
	cert, err := x509.ParseCertificate(cblk.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(kblk.Bytes)
	if err != nil {
		return nil, err
	}
	s256 := sha256.Sum256(cblk.Bytes)
	s1 := sha1.Sum(cblk.Bytes)
	return &CA{
		Cert: cert, Key: key, PEMPath: certPath,
		SHA256: hex.EncodeToString(s256[:]),
		SHA1:   strings.ToUpper(hex.EncodeToString(s1[:])),
	}, nil
}

func writePEM(path, typ string, der []byte, mode os.FileMode) error {
	b := pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
	if err := os.WriteFile(path, b, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func newSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}

func normalize(in []string) []string {
	in = append(append([]string{}, DefaultPermitted...), in...)
	for i := range in {
		in[i] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in[i]), "."))
	}
	return uniq(in)
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func subset(want, have []string) bool {
	h := map[string]bool{}
	for _, s := range have {
		h[s] = true
	}
	for _, s := range want {
		if !h[s] {
			return false
		}
	}
	return true
}
