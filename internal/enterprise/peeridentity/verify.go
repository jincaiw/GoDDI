// Package peeridentity binds a verified TLS peer to a locally configured node.
// It does not run a TLS handshake, check revocation, grant a role, or authorize replication. ConnectionState must be obtained directly
// from a correctly configured TLS connection; fabricated state is untrusted.
package peeridentity

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("peeridentity: invalid peer identity")

const (
	maxCertificates     = 16
	maxChains           = 16
	maxCertificateBytes = 256 << 10
	maxTotalDERBytes    = 4 << 20
)

// boundDER runs before parsing or copying certificate data, including policy
// roots and all chain entries; repeated entries count toward the same budget.
func boundDER(state tls.ConnectionState, p Policy) bool {
	if len(state.PeerCertificates) == 0 || len(state.PeerCertificates) > maxCertificates || len(state.VerifiedChains) == 0 || len(state.VerifiedChains) > maxChains || len(p.RootDER) == 0 || len(p.RootDER) > maxCertificates {
		return false
	}
	total := 0
	add := func(raw []byte) bool {
		if len(raw) == 0 || len(raw) > maxCertificateBytes || len(raw) > maxTotalDERBytes-total {
			return false
		}
		total += len(raw)
		return true
	}
	for _, raw := range p.RootDER {
		if !add(raw) {
			return false
		}
	}
	for _, cert := range state.PeerCertificates {
		if cert == nil || !add(cert.Raw) {
			return false
		}
	}
	for _, chain := range state.VerifiedChains {
		if len(chain) == 0 || len(chain) > maxCertificates {
			return false
		}
		for _, cert := range chain {
			if cert == nil || !add(cert.Raw) {
				return false
			}
		}
	}
	return true
}

type PinType uint8

const (
	CertificateSHA256 PinType = 1
	PublicKeySHA256   PinType = 2
)

// Policy is a locally configured binding, not data supplied by the peer. Exactly
// one of DNSName or SPIFFEURI is required, plus exactly one SHA-256 pin. The
// configured cluster/node pair is bound to that SAN and pin by local policy;
// a DNS SAN itself need not encode UUIDs. URI policy uses the explicit local
// profile spiffe://trust-domain/cluster/<cluster-uuid>/node/<node-uuid>.
type Policy struct {
	ClusterID string
	NodeID    string
	DNSName   string
	SPIFFEURI string
	PinType   PinType
	Pin       [32]byte
	// RootDER is independently configured trusted CA DER; never use roots from the peer.
	RootDER [][]byte
	// EKU must specify the expected TLS client or server purpose.
	EKU x509.ExtKeyUsage
}

// Identity contains only the configured identities and verified certificate
// fingerprint. It carries no authorization or claim about epoch or log state.
type Identity struct {
	ClusterID         string
	NodeID            string
	CertificateSHA256 [32]byte
}

// onlySAN rejects unrecognized GeneralName variants too, not merely those
// represented by crypto/x509's DNSNames/URIs/IPAddresses/EmailAddresses.
func onlySAN(leaf *x509.Certificate, wantTag int) bool {
	var value []byte
	for _, ext := range leaf.Extensions {
		if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) {
			if value != nil {
				return false
			}
			value = ext.Value
		}
	}
	var names []asn1.RawValue
	rest, err := asn1.Unmarshal(value, &names)
	return err == nil && len(rest) == 0 && len(names) == 1 && names[0].Class == 2 && names[0].Tag == wantTag && !names[0].IsCompound
}

func canonicalUUID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}
func dnsName(s string) bool {
	if len(s) == 0 || len(s) > 253 || strings.ToLower(s) != s || strings.HasSuffix(s, ".") {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func (p Policy) validate() error {
	if !canonicalUUID(p.ClusterID) || !canonicalUUID(p.NodeID) || p.ClusterID == p.NodeID || (p.DNSName == "") == (p.SPIFFEURI == "") || p.Pin == ([32]byte{}) || (p.PinType != CertificateSHA256 && p.PinType != PublicKeySHA256) || len(p.RootDER) == 0 || len(p.RootDER) > 16 || (p.EKU != x509.ExtKeyUsageClientAuth && p.EKU != x509.ExtKeyUsageServerAuth) {
		return ErrInvalid
	}
	if p.DNSName != "" && !dnsName(p.DNSName) {
		return ErrInvalid
	}
	if p.SPIFFEURI != "" {
		u, err := url.Parse(p.SPIFFEURI)
		if err != nil || u.Scheme != "spiffe" || !dnsName(u.Host) || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || u.Opaque != "" || u.Path != "/cluster/"+p.ClusterID+"/node/"+p.NodeID || u.String() != p.SPIFFEURI {
			return ErrInvalid
		}
	}
	return nil
}

// Verify uses the caller's explicit trusted UTC time; zero is invalid. TLS must
// have verified the peer chain in the correct trust domain with appropriate EKU
// and without insecure verification bypasses. This function independently
// verifies a chain against policy roots, time and EKU. VerifiedChains is not an
// independently authenticated proof if ConnectionState was constructed by code.
func Verify(state tls.ConnectionState, p Policy, trustedUTC time.Time) (Identity, error) {
	if !state.HandshakeComplete {
		return Identity{}, ErrInvalid
	}
	return verifyCertificate(state, p, trustedUTC)
}

// VerifyHandshake is only for a Go TLS VerifyConnection callback. The caller
// must use Go's normal verified-chain path, with InsecureSkipVerify disabled
// and appropriate client/server certificate verification configured. A caller
// can fabricate ConnectionState; this function cannot prove the state came from
// TLS or that default verification was enabled. Callback success identifies a
// peer candidate, not a completed handshake, authorized role, trusted clock,
// fencing, or permission to replicate.
func VerifyHandshake(state tls.ConnectionState, p Policy, trustedUTC time.Time) (Identity, error) {
	if state.Version != tls.VersionTLS13 {
		return Identity{}, ErrInvalid
	}
	return verifyCertificate(state, p, trustedUTC)
}

func verifyCertificate(state tls.ConnectionState, p Policy, trustedUTC time.Time) (Identity, error) {
	var zero Identity
	if p.validate() != nil || trustedUTC.IsZero() || !boundDER(state, p) {
		return zero, ErrInvalid
	}
	peer := state.PeerCertificates[0]
	if len(peer.Raw) == 0 {
		return zero, ErrInvalid
	}
	// Parse DER again instead of trusting potentially inconsistent exposed fields.
	leaf, err := x509.ParseCertificate(peer.Raw)
	if err != nil || len(leaf.UnhandledCriticalExtensions) != 0 {
		return zero, ErrInvalid
	}
	// x509.Verify treats missing EKU and ExtKeyUsageAny as unrestricted.
	// Node certificates must explicitly carry the expected purpose instead.
	hasPurpose := false
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageAny {
			return zero, ErrInvalid
		}
		if usage == p.EKU {
			hasPurpose = true
		}
	}
	if !hasPurpose {
		return zero, ErrInvalid
	}
	fp := sha256.Sum256(leaf.Raw)
	chainOK := false
	for _, chain := range state.VerifiedChains {
		if len(chain) == 0 || chain[0] == nil || len(chain[0].Raw) == 0 || sha256.Sum256(chain[0].Raw) != fp || !bytes.Equal(chain[0].Raw, leaf.Raw) {
			continue
		}
		valid := true
		for _, entry := range chain {
			if entry == nil || len(entry.Raw) == 0 {
				valid = false
				break
			}
			cert, err := x509.ParseCertificate(entry.Raw)
			if err != nil || len(cert.UnhandledCriticalExtensions) != 0 || trustedUTC.Before(cert.NotBefore) || trustedUTC.After(cert.NotAfter) {
				valid = false
				break
			}
		}
		if valid {
			chainOK = true
			break
		}
	}
	if !chainOK {
		return zero, fmt.Errorf("%w: missing matching verified chain or invalid time", ErrInvalid)
	}
	roots := x509.NewCertPool()
	for _, raw := range p.RootDER {
		if len(raw) == 0 || len(raw) > 1<<20 {
			return zero, ErrInvalid
		}
		ca, err := x509.ParseCertificate(raw)
		if err != nil || !ca.IsCA {
			return zero, ErrInvalid
		}
		roots.AddCert(ca)
	}
	intermediates := x509.NewCertPool()
	for _, cert := range state.PeerCertificates[1:] {
		if cert == nil || len(cert.Raw) == 0 {
			return zero, ErrInvalid
		}
		parsed, err := x509.ParseCertificate(cert.Raw)
		if err != nil {
			return zero, ErrInvalid
		}
		intermediates.AddCert(parsed)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: trustedUTC, KeyUsages: []x509.ExtKeyUsage{p.EKU}}); err != nil {
		return zero, fmt.Errorf("%w: independent chain verification", ErrInvalid)
	}
	// No ambiguous identities or wildcard matching; CN is never considered.
	if len(leaf.EmailAddresses) != 0 || len(leaf.IPAddresses) != 0 {
		return zero, ErrInvalid
	}
	if p.DNSName != "" {
		if !onlySAN(leaf, 2) || len(leaf.DNSNames) != 1 || len(leaf.URIs) != 0 || leaf.DNSNames[0] != p.DNSName {
			return zero, ErrInvalid
		}
	} else {
		if !onlySAN(leaf, 6) || len(leaf.URIs) != 1 || len(leaf.DNSNames) != 0 || leaf.URIs[0] == nil || leaf.URIs[0].String() != p.SPIFFEURI {
			return zero, ErrInvalid
		}
	}
	actual := fp
	if p.PinType == PublicKeySHA256 {
		actual = sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	}
	if actual != p.Pin {
		return zero, fmt.Errorf("%w: pin", ErrInvalid)
	}
	return Identity{ClusterID: p.ClusterID, NodeID: p.NodeID, CertificateSHA256: fp}, nil
}
