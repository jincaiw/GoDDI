package hapermit

import "fmt"

// Expected is independently acquired from a trusted local preview and registry.
// It is not a request claim and does not prove that those sources are valid.
// A seq=0 anchor can inherit a zero hash across empty epochs.
type Expected struct {
	ClusterID, PredecessorID, CandidateID string
	OldEpoch, NewEpoch                    int64
	AnchorEpoch, AnchorSeq                int64
	AnchorHash                            [32]byte
	PlanDigest, FencingDigest             [32]byte
	Nonce, Requester                      string
}

// Claims holds verified document values without referencing input slices or maps.
// It is not a grant, fencing proof, registry verification or nonce consumption.
type Claims struct {
	Expected
	NotBeforeMS, NotAfterMS int64
}

// VerifyExpected verifies signatures and time before decoding verified claims,
// then requires an exact match to independently sourced local expectations.
// The caller must prevent concurrent mutation of inputs during the call. The
// returned values alone never authorize a role change or establish fencing.
func VerifyExpected(body []byte, approvals []Approval, keyring map[string]Approver, now Bounds, expected Expected) (Claims, error) {
	var zero Claims
	if len(body) == 0 || len(body) > MaxBodyBytes {
		return zero, ErrInvalid
	}
	// One private snapshot is used for signature verification and extraction.
	snapshot := append([]byte(nil), body...)
	if err := Verify(snapshot, approvals, keyring, now); err != nil {
		return zero, err
	}
	v, err := parse(snapshot)
	if err != nil {
		return zero, ErrInvalid
	}
	old, err := integer(v["oldEpoch"], true)
	if err != nil {
		return zero, ErrInvalid
	}
	next, err := integer(v["newEpoch"], true)
	if err != nil {
		return zero, ErrInvalid
	}
	aepoch, err := integer(v["anchorEpoch"], true)
	if err != nil {
		return zero, ErrInvalid
	}
	seq, err := integer(v["anchorSeq"], false)
	if err != nil {
		return zero, ErrInvalid
	}
	before, err := integer(v["notBeforeMs"], false)
	if err != nil {
		return zero, ErrInvalid
	}
	after, err := integer(v["notAfterMs"], false)
	if err != nil {
		return zero, ErrInvalid
	}
	anchor, err := binary(v["anchorHash"], 32)
	if err != nil {
		return zero, ErrInvalid
	}
	plan, err := binary(v["planDigest"], 32)
	if err != nil {
		return zero, ErrInvalid
	}
	fence, err := binary(v["fencingDigest"], 32)
	if err != nil {
		return zero, ErrInvalid
	}
	claims := Claims{Expected: Expected{ClusterID: v["clusterId"], PredecessorID: v["predecessorId"], CandidateID: v["candidateId"], OldEpoch: old, NewEpoch: next, AnchorEpoch: aepoch, AnchorSeq: seq, Nonce: v["nonce"], Requester: v["requester"]}, NotBeforeMS: before, NotAfterMS: after}
	copy(claims.AnchorHash[:], anchor)
	copy(claims.PlanDigest[:], plan)
	copy(claims.FencingDigest[:], fence)
	if claims.Expected != expected {
		return zero, fmt.Errorf("%w: local context mismatch", ErrInvalid)
	}
	return claims, nil
}
