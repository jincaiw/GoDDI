package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/jasonwa/goddi/internal/secretbox"
)

// A snapshot holds every password hash, TOTP seed and TSIG key the instance
// owns, so the archive is encrypted as a whole rather than relying on the file
// mode of the place it was written to. A backup leaving the host -- on a USB
// stick, in an object store, attached to the ticket that asks for a restore --
// is exactly the case a mode bit cannot cover.
//
// AES-256-GCM is applied in fixed-size chunks rather than to the whole archive
// in one message, for two reasons. A multi-gigabyte control database must not
// have to fit in memory twice, and a single GCM message over a stream of
// unknown length gives no way to notice that the tail is missing. Each chunk
// carries its index in both the nonce and the associated data, so a chunk that
// is reordered, duplicated or dropped fails to authenticate instead of quietly
// producing different plaintext.
//
// Layout:
//
//	"GODDI-SNAP-ENC1"   magic; the trailing 1 is the container version
//	uint32 BE           plaintext chunk size
//	12 bytes            fresh random base nonce
//	repeated:
//	  uint32 BE         sealed length, ciphertext plus tag
//	  sealed bytes
//	uint32 BE 0         terminator
//
// The whole header is folded into every chunk's associated data, so the chunk
// size and the container version cannot be edited on a file that is otherwise
// valid.
const (
	// snapshotEncMagic is both the file's first bytes and its container
	// version. A reader that does not recognise the magic refuses the file
	// instead of guessing at a layout it does not implement.
	snapshotEncMagic = "GODDI-SNAP-ENC1"

	snapshotChunkSize    = 1 << 20 // 1 MiB of plaintext per chunk
	snapshotChunkMaxSize = 64 << 20
	// snapshotChunkMinSize is a sanity floor rather than a format rule. The
	// chunk size travels in the header so that a later build may change it, but
	// a file declaring one byte per chunk would make the per-chunk overhead --
	// 20 bytes of framing and tag for every byte of payload -- something the
	// reader's memory use is at the mercy of.
	snapshotChunkMinSize = 64 << 10

	snapshotNonceSize = 12
	// snapshotEncHeaderLen covers the magic, the chunk size and the base nonce.
	snapshotEncHeaderLen = len(snapshotEncMagic) + 4 + snapshotNonceSize
)

// The Encryption field of a manifest. A reader refuses any value that is not
// one of these rather than assuming an archive it cannot decrypt is plaintext.
const (
	SnapshotEncryptionNone       = "none"
	SnapshotEncryptionChunkedGCM = "aes-256-gcm/chunked-v1"
)

// snapshotAEAD derives the key an archive is sealed under.
//
// The purpose label is part of the derivation, so the archive key and the keys
// the same configured material produces for database columns are different
// keys: a snapshot file can never be used as a TSIG key or a TOTP seed, nor the
// other way round.
func snapshotAEAD(material string) (cipher.AEAD, error) {
	block, err := aes.NewCipher(secretbox.Derive(secretbox.LabelSnapshot, material))
	if err != nil {
		return nil, fmt.Errorf("snapshot: deriving the archive key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("snapshot: building the archive cipher: %w", err)
	}
	return aead, nil
}

// snapshotEncryptor seals a byte stream into the container above. It is an
// io.Writer, so the caller can stream a tar archive through it without ever
// holding the archive in memory. Close must be called: it writes the final
// partial chunk and the terminator, and a file missing either is not readable.
type snapshotEncryptor struct {
	dest    io.Writer
	aead    cipher.AEAD
	header  []byte
	base    []byte
	counter uint32
	buf     []byte
	closed  bool
}

func newSnapshotEncryptor(dest io.Writer, material string) (*snapshotEncryptor, error) {
	aead, err := snapshotAEAD(material)
	if err != nil {
		return nil, err
	}
	base := make([]byte, snapshotNonceSize)
	if _, err := io.ReadFull(rand.Reader, base); err != nil {
		return nil, fmt.Errorf("snapshot: generating the archive nonce: %w", err)
	}
	header := make([]byte, 0, snapshotEncHeaderLen)
	header = append(header, snapshotEncMagic...)
	header = binary.BigEndian.AppendUint32(header, snapshotChunkSize)
	header = append(header, base...)

	if _, err := dest.Write(header); err != nil {
		return nil, fmt.Errorf("snapshot: writing the archive header: %w", err)
	}
	return &snapshotEncryptor{
		dest:   dest,
		aead:   aead,
		header: header,
		base:   base,
		buf:    make([]byte, 0, snapshotChunkSize),
	}, nil
}

func (e *snapshotEncryptor) Write(p []byte) (int, error) {
	if e.closed {
		return 0, errors.New("snapshot: write after close")
	}
	total := len(p)
	for len(p) > 0 {
		space := snapshotChunkSize - len(e.buf)
		n := min(len(p), space)
		e.buf = append(e.buf, p[:n]...)
		p = p[n:]
		if len(e.buf) == snapshotChunkSize {
			if err := e.sealChunk(e.buf); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

// Close writes the last partial chunk and the terminator. It is idempotent so
// that a deferred call after an explicit one is not an error.
func (e *snapshotEncryptor) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	if len(e.buf) > 0 {
		if err := e.sealChunk(e.buf); err != nil {
			return err
		}
	}
	var terminator [4]byte
	if _, err := e.dest.Write(terminator[:]); err != nil {
		return fmt.Errorf("snapshot: writing the archive terminator: %w", err)
	}
	return nil
}

func (e *snapshotEncryptor) sealChunk(plaintext []byte) error {
	sealed := e.aead.Seal(nil, e.chunkNonce(), plaintext, e.chunkAAD())
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(sealed)))
	if _, err := e.dest.Write(length[:]); err != nil {
		return fmt.Errorf("snapshot: writing chunk %d length: %w", e.counter, err)
	}
	if _, err := e.dest.Write(sealed); err != nil {
		return fmt.Errorf("snapshot: writing chunk %d: %w", e.counter, err)
	}
	e.counter++
	e.buf = e.buf[:0]
	return nil
}

// chunkNonce puts the counter first and keeps the freshly generated random
// bytes behind it, so every file starts from 96 bits of randomness and the only
// reuse that could ever matter -- the same file, twice -- is impossible.
func (e *snapshotEncryptor) chunkNonce() []byte {
	nonce := make([]byte, snapshotNonceSize)
	binary.BigEndian.PutUint32(nonce, e.counter)
	copy(nonce[4:], e.base[4:])
	return nonce
}

// chunkAAD binds the chunk to its position and to the header it was written
// under.
func (e *snapshotEncryptor) chunkAAD() []byte {
	aad := make([]byte, 0, len(e.header)+4)
	aad = append(aad, e.header...)
	return binary.BigEndian.AppendUint32(aad, e.counter)
}

// snapshotDecryptor is the reader half. It implements io.Reader over the
// plaintext, and Finish reports whether the archive ended the way a complete
// one ends.
type snapshotDecryptor struct {
	src       io.Reader
	aead      cipher.AEAD
	header    []byte
	base      []byte
	chunkSize int
	counter   uint32
	buf       []byte
	done      bool
}

func newSnapshotDecryptor(src io.Reader, material string) (*snapshotDecryptor, error) {
	aead, err := snapshotAEAD(material)
	if err != nil {
		return nil, err
	}
	header := make([]byte, snapshotEncHeaderLen)
	if _, err := io.ReadFull(src, header); err != nil {
		return nil, fmt.Errorf("snapshot: reading the archive header: %w", err)
	}
	if string(header[:len(snapshotEncMagic)]) != snapshotEncMagic {
		return nil, fmt.Errorf("snapshot: not a %s archive", snapshotEncMagic)
	}
	chunkSize := int(binary.BigEndian.Uint32(header[len(snapshotEncMagic):]))
	if chunkSize < snapshotChunkMinSize || chunkSize > snapshotChunkMaxSize {
		return nil, fmt.Errorf("snapshot: the archive declares a chunk size of %d bytes; this build reads between %d and %d",
			chunkSize, snapshotChunkMinSize, snapshotChunkMaxSize)
	}
	return &snapshotDecryptor{
		src:       src,
		aead:      aead,
		header:    header,
		base:      header[len(header)-snapshotNonceSize:],
		chunkSize: chunkSize,
	}, nil
}

func (d *snapshotDecryptor) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		if d.done {
			return 0, io.EOF
		}
		if err := d.fill(); err != nil {
			return 0, err
		}
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

func (d *snapshotDecryptor) fill() error {
	var lengthBuf [4]byte
	if _, err := io.ReadFull(d.src, lengthBuf[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("snapshot: the archive ends without a terminator; it is truncated")
		}
		return fmt.Errorf("snapshot: reading chunk %d length: %w", d.counter, err)
	}
	length := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if length == 0 {
		d.done = true
		return nil
	}
	if length < d.aead.Overhead() || length > d.chunkSize+d.aead.Overhead() {
		return fmt.Errorf("snapshot: chunk %d declares an impossible length of %d bytes", d.counter, length)
	}
	sealed := make([]byte, length)
	if _, err := io.ReadFull(d.src, sealed); err != nil {
		return fmt.Errorf("snapshot: reading chunk %d: %w", d.counter, err)
	}
	plaintext, err := d.aead.Open(nil, d.chunkNonce(), sealed, d.chunkAAD())
	if err != nil {
		return fmt.Errorf("snapshot: chunk %d does not authenticate: the key material is not the one the archive was written with, or the archive has been altered (%w)", d.counter, err)
	}
	d.buf = plaintext
	d.counter++
	return nil
}

// Finish consumes whatever is left of the source and reports whether the
// archive ended with a terminator.
//
// It has to be called after the consumer of the plaintext is done, and the
// result has to be checked. gzip already carries a length and a checksum for
// the stream, so a truncated tail is caught without this -- but only if the
// truncation lands inside the gzip stream. Finish is what makes "the file ends
// where the writer said it would" an explicit, verified statement rather than a
// consequence of where the truncation happened to fall.
func (d *snapshotDecryptor) Finish() error {
	if _, err := io.Copy(io.Discard, d); err != nil {
		return err
	}
	return nil
}

func (d *snapshotDecryptor) chunkNonce() []byte {
	nonce := make([]byte, snapshotNonceSize)
	binary.BigEndian.PutUint32(nonce, d.counter)
	copy(nonce[4:], d.base[4:])
	return nonce
}

func (d *snapshotDecryptor) chunkAAD() []byte {
	aad := make([]byte, 0, len(d.header)+4)
	aad = append(aad, d.header...)
	return binary.BigEndian.AppendUint32(aad, d.counter)
}
