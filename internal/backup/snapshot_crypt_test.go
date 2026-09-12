package backup

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

// encryptForTest runs a payload through the container so a test can look at the
// bytes it produced.
func encryptForTest(t *testing.T, plaintext []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	encryptor, err := newSnapshotEncryptor(&buf, snapshotTestKey)
	if err != nil {
		t.Fatalf("building the encryptor: %v", err)
	}
	if _, err := encryptor.Write(plaintext); err != nil {
		t.Fatalf("sealing the payload: %v", err)
	}
	if err := encryptor.Close(); err != nil {
		t.Fatalf("closing the encryptor: %v", err)
	}
	return buf.Bytes()
}

func decryptForTest(t *testing.T, sealed []byte, material string) ([]byte, error) {
	t.Helper()
	decryptor, err := newSnapshotDecryptor(bytes.NewReader(sealed), material)
	if err != nil {
		return nil, err
	}
	plaintext, err := io.ReadAll(decryptor)
	if err != nil {
		return nil, err
	}
	return plaintext, decryptor.Finish()
}

// sealedChunks lists the sealed length of each chunk, terminator excluded, so a
// test can assert how a payload was divided.
func sealedChunks(t *testing.T, sealed []byte) []int {
	t.Helper()
	if !bytes.HasPrefix(sealed, []byte(snapshotEncMagic)) {
		t.Fatalf("the container does not start with %q", snapshotEncMagic)
	}
	chunkSize := int(binary.BigEndian.Uint32(sealed[len(snapshotEncMagic):]))
	if chunkSize != snapshotChunkSize {
		t.Fatalf("the container declares a chunk size of %d, want %d", chunkSize, snapshotChunkSize)
	}
	body := sealed[snapshotEncHeaderLen:]
	var lengths []int
	for len(body) > 0 {
		if len(body) < 4 {
			t.Fatalf("%d bytes left over where a chunk length should be", len(body))
		}
		length := int(binary.BigEndian.Uint32(body))
		body = body[4:]
		if length == 0 {
			if len(body) != 0 {
				t.Errorf("%d bytes follow the terminator", len(body))
			}
			break
		}
		if length > len(body) {
			t.Fatalf("chunk declares %d bytes but only %d remain", length, len(body))
		}
		lengths = append(lengths, length)
		body = body[length:]
	}
	return lengths
}

func TestTheContainerRoundTripsAPayloadSpanningSeveralChunks(t *testing.T) {
	payload := make([]byte, snapshotChunkSize*2+12345)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	sealed := encryptForTest(t, payload)
	plaintext, err := decryptForTest(t, sealed, snapshotTestKey)
	if err != nil {
		t.Fatalf("opening the container: %v", err)
	}
	if !bytes.Equal(plaintext, payload) {
		t.Fatalf("the container returned %d bytes that differ from the %d it was given", len(plaintext), len(payload))
	}

	chunks := sealedChunks(t, sealed)
	if len(chunks) != 3 {
		t.Fatalf("the payload was written as %d chunks, want 3", len(chunks))
	}
	if chunks[0] != snapshotChunkSize+16 || chunks[1] != snapshotChunkSize+16 {
		t.Errorf("full chunks are %v, want %d + 16 bytes of tag each", chunks[:2], snapshotChunkSize)
	}
	if chunks[2] != 12345+16 {
		t.Errorf("the last chunk is %d bytes, want 12345 + 16", chunks[2])
	}
	if len(sealed) <= len(payload) {
		t.Errorf("the container is %d bytes for %d of payload: it is not encrypting anything", len(sealed), len(payload))
	}
}

func TestAPayloadOfExactlyOneChunkIsWrittenAsOneChunk(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), snapshotChunkSize)
	sealed := encryptForTest(t, payload)

	chunks := sealedChunks(t, sealed)
	if len(chunks) != 1 {
		t.Fatalf("the payload was written as %d chunks, want 1: a full chunk must not be followed by an empty one", len(chunks))
	}
	plaintext, err := decryptForTest(t, sealed, snapshotTestKey)
	if err != nil {
		t.Fatalf("opening the container: %v", err)
	}
	if !bytes.Equal(plaintext, payload) {
		t.Error("the container did not return the payload it was given")
	}
}

func TestAnEmptyPayloadRoundTrips(t *testing.T) {
	sealed := encryptForTest(t, nil)
	if chunks := sealedChunks(t, sealed); len(chunks) != 0 {
		t.Fatalf("an empty payload was written as %d chunks", len(chunks))
	}
	plaintext, err := decryptForTest(t, sealed, snapshotTestKey)
	if err != nil {
		t.Fatalf("opening an empty container: %v", err)
	}
	if len(plaintext) != 0 {
		t.Errorf("an empty container returned %d bytes", len(plaintext))
	}
}

func TestTheContainerRefusesKeyMaterialItWasNotWrittenWith(t *testing.T) {
	sealed := encryptForTest(t, []byte("a payload"))
	if _, err := decryptForTest(t, sealed, "some-other-material"); err == nil {
		t.Fatal("the container opened with the wrong key material")
	}

	// Changing a single byte of the header must be caught as well: the header
	// is folded into every chunk's associated data, so the chunk size and the
	// container version cannot be edited on an otherwise valid file.
	edited := append([]byte(nil), sealed...)
	edited[len(snapshotEncMagic)+3] ^= 0x01
	if _, err := decryptForTest(t, edited, snapshotTestKey); err == nil {
		t.Fatal("the container opened after its declared chunk size was changed")
	}
}

func TestAChangedByteInTheBodyIsRefused(t *testing.T) {
	sealed := encryptForTest(t, []byte("a payload long enough to matter"))
	edited := append([]byte(nil), sealed...)
	edited[len(edited)-5] ^= 0x01

	_, err := decryptForTest(t, edited, snapshotTestKey)
	if err == nil {
		t.Fatal("the container opened after a byte of the body was changed")
	}
	if !strings.Contains(err.Error(), "does not authenticate") {
		t.Errorf("the refusal does not say what failed: %v", err)
	}
}

// TestChunksCannotBeSwapped checks the counter in the nonce and the associated
// data: without it, two chunks of the same length could be exchanged and the
// payload would come back as different, plausible-looking plaintext.
func TestChunksCannotBeSwapped(t *testing.T) {
	payload := make([]byte, snapshotChunkSize*2)
	for i := range payload {
		payload[i] = byte(i % 97)
	}
	sealed := encryptForTest(t, payload)
	chunks := sealedChunks(t, sealed)
	if len(chunks) != 2 {
		t.Fatalf("the payload was written as %d chunks, want 2", len(chunks))
	}

	body := sealed[snapshotEncHeaderLen:]
	first := append([]byte(nil), body[4:4+chunks[0]]...)
	secondStart := 4 + chunks[0]
	second := append([]byte(nil), body[secondStart+4:secondStart+4+chunks[1]]...)

	swapped := append([]byte(nil), sealed[:snapshotEncHeaderLen]...)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(second)))
	swapped = append(swapped, length[:]...)
	swapped = append(swapped, second...)
	binary.BigEndian.PutUint32(length[:], uint32(len(first)))
	swapped = append(swapped, length[:]...)
	swapped = append(swapped, first...)
	swapped = append(swapped, 0, 0, 0, 0)

	_, err := decryptForTest(t, swapped, snapshotTestKey)
	if err == nil {
		t.Fatal("two chunks were exchanged and the container still opened")
	}
	if !strings.Contains(err.Error(), "chunk 0") {
		t.Errorf("the refusal does not say which chunk failed first: %v", err)
	}
}

// TestATruncatedContainerSaysSo covers the case a single GCM message over a
// stream cannot express: the last chunk is intact and simply missing.
func TestATruncatedContainerSaysSo(t *testing.T) {
	payload := make([]byte, snapshotChunkSize+4096)
	sealed := encryptForTest(t, payload)

	withoutTerminator := sealed[:len(sealed)-4]
	_, err := decryptForTest(t, withoutTerminator, snapshotTestKey)
	if err == nil {
		t.Fatal("a container whose terminator was removed still opened")
	}
	if !strings.Contains(err.Error(), "terminator") {
		t.Errorf("the refusal does not say the archive is truncated: %v", err)
	}

	// A file that stops in the middle of a chunk must be refused too.
	halfChunk := sealed[:len(sealed)-snapshotChunkSize/2]
	if _, err := decryptForTest(t, halfChunk, snapshotTestKey); err == nil {
		t.Fatal("a container that stops mid-chunk still opened")
	}
}

func TestAContainerThatIsNotOneIsRefusedByName(t *testing.T) {
	notAContainer := append(bytes.Repeat([]byte("x"), snapshotEncHeaderLen), 0, 0, 0, 0)
	_, err := newSnapshotDecryptor(bytes.NewReader(notAContainer), snapshotTestKey)
	if err == nil {
		t.Fatal("a blob of garbage was accepted as a container")
	}
	if !strings.Contains(err.Error(), snapshotEncMagic) {
		t.Errorf("the refusal does not name what it expected: %v", err)
	}
}

func TestDeclaredChunkSizesThisBuildDoesNotImplementAreRefused(t *testing.T) {
	for _, size := range []uint32{0, 1, snapshotChunkMaxSize + 1} {
		header := append([]byte(nil), snapshotEncMagic...)
		header = binary.BigEndian.AppendUint32(header, size)
		header = append(header, make([]byte, snapshotNonceSize)...)
		_, err := newSnapshotDecryptor(bytes.NewReader(header), snapshotTestKey)
		if err == nil {
			t.Errorf("a container declaring a chunk size of %d was accepted", size)
		}
	}
}
