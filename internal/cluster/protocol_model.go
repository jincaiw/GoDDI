package cluster

import "fmt"

// The protocol model is an executable contract for ADR-0005. It deliberately
// has no network, storage, election, or HTTP integration; ErrNotImplemented
// remains the production API boundary until those pieces exist.
type ProtocolModel struct {
	epoch      int64
	leader     string
	quorum     int
	votes      int
	nextSeq    int64
	nextRev    int64
	publishes  map[string]PublishRecord
	watermarks map[string]int64
	fenced     map[string]bool
	cleaned    map[string]bool
}

type PublishRequest struct {
	PublishID string
	Epoch     int64
	Leader    string
}

type PublishRecord struct {
	PublishID string
	Epoch     int64
	Revision  int64
	Seq       int64
	Status    string
}

const (
	PublishCommitted = "committed"
	PublishUnknown   = "unknown"
)

func NewProtocolModel(epoch int64, leader string, quorum int) *ProtocolModel {
	if quorum < 1 {
		quorum = 1
	}
	return &ProtocolModel{
		epoch:      epoch,
		leader:     leader,
		quorum:     quorum,
		publishes:  make(map[string]PublishRecord),
		watermarks: make(map[string]int64),
		fenced:     make(map[string]bool),
		cleaned:    make(map[string]bool),
	}
}

func (m *ProtocolModel) SetVotes(votes int) { m.votes = votes }

func (m *ProtocolModel) Publish(req PublishRequest) (PublishRecord, error) {
	if req.PublishID == "" {
		return PublishRecord{}, fmt.Errorf("cluster protocol: publish_id is required")
	}
	if existing, ok := m.publishes[req.PublishID]; ok {
		return existing, nil
	}
	if req.Epoch != m.epoch {
		return PublishRecord{}, fmt.Errorf("cluster protocol: stale epoch")
	}
	if req.Leader != m.leader {
		return PublishRecord{}, fmt.Errorf("cluster protocol: leader is not authorized")
	}
	if m.votes < m.quorum {
		return PublishRecord{}, fmt.Errorf("cluster protocol: quorum is not available")
	}
	m.nextSeq++
	m.nextRev++
	record := PublishRecord{
		PublishID: req.PublishID,
		Epoch:     req.Epoch,
		Revision:  m.nextRev,
		Seq:       m.nextSeq,
		Status:    PublishCommitted,
	}
	m.publishes[req.PublishID] = record
	return record, nil
}

func (m *ProtocolModel) MarkUnknown(publishID string) error {
	record, ok := m.publishes[publishID]
	if !ok {
		return fmt.Errorf("cluster protocol: publish not found")
	}
	record.Status = PublishUnknown
	m.publishes[publishID] = record
	return nil
}

func (m *ProtocolModel) Query(publishID string) (PublishRecord, bool) {
	record, ok := m.publishes[publishID]
	return record, ok
}

func (m *ProtocolModel) Apply(node string, seq int64) error {
	applied := m.watermarks[node]
	switch {
	case seq <= applied:
		return nil
	case seq != applied+1:
		return fmt.Errorf("cluster protocol: watermark gap")
	default:
		m.watermarks[node] = seq
		return nil
	}
}

func (m *ProtocolModel) FenceNode(node string) {
	m.fenced[node] = true
	m.cleaned[node] = false
}

func (m *ProtocolModel) CleanNode(node string) {
	m.cleaned[node] = true
}

func (m *ProtocolModel) RejoinNode(node string) error {
	if !m.fenced[node] {
		return fmt.Errorf("cluster protocol: node is not fenced")
	}
	if !m.cleaned[node] {
		return fmt.Errorf("cluster protocol: node has unconfirmed state")
	}
	m.fenced[node] = false
	return nil
}
