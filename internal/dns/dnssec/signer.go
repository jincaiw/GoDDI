package dnssec

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sort"

	"github.com/miekg/dns"
)

// SignRRset signs an RRset with a DNSSEC key and returns the RRSIG record.
func SignRRset(rrset []dns.RR, key *DNSSECKey, signer crypto.Signer, zoneName string, inception, expiration int64) (*dns.RRSIG, error) {
	if len(rrset) == 0 {
		return nil, fmt.Errorf("empty RRset")
	}

	algoNum, ok := SupportedAlgorithms[DNSSECAlgorithm(key.Algorithm)]
	if !ok {
		return nil, fmt.Errorf("unsupported algorithm: %s", key.Algorithm)
	}

	rrsig := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: rrset[0].Header().Name, Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: rrset[0].Header().Ttl},
		TypeCovered: rrset[0].Header().Rrtype,
		Algorithm:   algoNum,
		Labels:      uint8(dns.CountLabel(rrset[0].Header().Name)),
		OrigTtl:     rrset[0].Header().Ttl,
		Expiration:  uint32(expiration),
		Inception:   uint32(inception),
		KeyTag:      uint16(key.KeyTag),
		SignerName:  zoneName,
	}

	err := rrsig.Sign(signer, rrset)
	if err != nil {
		return nil, fmt.Errorf("signing RRset: %w", err)
	}

	return rrsig, nil
}

// CreateNSECRecords creates NSEC records for a zone.
func CreateNSECRecords(zoneName string, records []dns.RR) ([]dns.NSEC, error) {
	if len(records) == 0 {
		return nil, nil
	}

	// Group records by name.
	nameSet := make(map[string][]uint16)
	for _, rr := range records {
		name := rr.Header().Name
		nameSet[name] = append(nameSet[name], rr.Header().Rrtype)
	}

	// Sort names.
	var names []string
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	// Create NSEC records.
	var nsecRecords []dns.NSEC
	for i, name := range names {
		nextName := zoneName
		if i+1 < len(names) {
			nextName = names[i+1]
		}

		types := nameSet[name]
		types = append(types, dns.TypeNSEC)
		typeMap := make(map[uint16]bool)
		for _, t := range types {
			typeMap[t] = true
		}

		nsec := dns.NSEC{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeNSEC,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			NextDomain: nextName,
			TypeBitMap: typesFromMap(typeMap),
		}
		nsecRecords = append(nsecRecords, nsec)
	}

	return nsecRecords, nil
}

// typesFromMap converts a type map to a sorted slice.
func typesFromMap(m map[uint16]bool) []uint16 {
	var types []uint16
	for t := range m {
		types = append(types, t)
	}
	for i := 0; i < len(types); i++ {
		for j := i + 1; j < len(types); j++ {
			if types[i] > types[j] {
				types[i], types[j] = types[j], types[i]
			}
		}
	}
	return types
}

// encryptAESGCM encrypts data using AES-256-GCM.
func encryptAESGCM(plaintext []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptAESGCM decrypts data using AES-256-GCM.
func decryptAESGCM(ciphertext string, key []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
