package forwarder

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// ConditionalForwarder maps domain suffixes to specific forwarder groups.
type ConditionalForwarder struct {
	ID           string   `json:"id"`
	Domain       string   `json:"domain"`
	ForwarderIDs []string `json:"forwarder_ids"`
	Enabled      bool     `json:"enabled"`

	forwarders []*Forwarder
	group      *ForwarderGroup
}

// ConditionalForwarderManager manages conditional forwarding rules.
type ConditionalForwarderManager struct {
	mu           sync.RWMutex
	conditionals []*ConditionalForwarder
	defaultGroup *ForwarderGroup
}

// NewConditionalForwarderManager creates a new conditional forwarder manager.
func NewConditionalForwarderManager(defaultGroup *ForwarderGroup) *ConditionalForwarderManager {
	return &ConditionalForwarderManager{
		conditionals: make([]*ConditionalForwarder, 0),
		defaultGroup: defaultGroup,
	}
}

// SetConditionals replaces the list of conditional forwarders.
func (m *ConditionalForwarderManager) SetConditionals(conditionals []*ConditionalForwarder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conditionals = conditionals
}

// AddConditional adds a conditional forwarder.
func (m *ConditionalForwarderManager) AddConditional(cf *ConditionalForwarder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conditionals = append(m.conditionals, cf)
}

// RemoveConditional removes a conditional forwarder by ID.
func (m *ConditionalForwarderManager) RemoveConditional(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, cf := range m.conditionals {
		if cf.ID == id {
			m.conditionals = append(m.conditionals[:i], m.conditionals[i+1:]...)
			return
		}
	}
}

// GetConditionals returns all conditional forwarders.
func (m *ConditionalForwarderManager) GetConditionals() []*ConditionalForwarder {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*ConditionalForwarder, len(m.conditionals))
	copy(result, m.conditionals)
	return result
}

// Forward forwards a DNS query using conditional forwarding rules.
// It matches the query name against domain suffixes and uses the
// appropriate forwarder group. Longest suffix match wins.
func (m *ConditionalForwarderManager) Forward(ctx context.Context, msg *dns.Msg) (*dns.Msg, *Forwarder, time.Duration, error) {
	if len(msg.Question) == 0 {
		return m.defaultGroup.Forward(ctx, msg)
	}

	qname := strings.ToLower(msg.Question[0].Name)

	// Find the best matching conditional forwarder (longest suffix match).
	group := m.matchForwarderGroup(qname)
	return group.Forward(ctx, msg)
}

// matchForwarderGroup finds the forwarder group for the given query name.
// Returns the default group if no conditional forwarder matches.
func (m *ConditionalForwarderManager) matchForwarderGroup(qname string) *ForwarderGroup {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var bestMatch *ConditionalForwarder
	bestLen := 0

	for _, cf := range m.conditionals {
		if !cf.Enabled {
			continue
		}

		domain := strings.ToLower(cf.Domain)
		// Remove trailing dots for comparison.
		domain = strings.TrimSuffix(domain, ".")
		qnameClean := strings.TrimSuffix(qname, ".")

		if matchDomain(domain, qnameClean) {
			if len(domain) > bestLen {
				bestLen = len(domain)
				bestMatch = cf
			}
		}
	}

	if bestMatch != nil && bestMatch.group != nil {
		return bestMatch.group
	}

	return m.defaultGroup
}

// matchDomain checks if qname matches the domain pattern.
// Supports exact match, suffix match, and wildcard.
func matchDomain(pattern, qname string) bool {
	// Exact match.
	if pattern == qname {
		return true
	}

	// Wildcard match (e.g., *.example.com).
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // Remove the *
		// qname must end with the suffix and have at least one label before.
		if strings.HasSuffix(qname, suffix) {
			prefix := qname[:len(qname)-len(suffix)]
			// Ensure there's at least one character before the suffix.
			return len(prefix) > 0
		}
		return false
	}

	// Suffix match: pattern "example.com" matches "sub.example.com".
	if strings.HasSuffix(qname, "."+pattern) {
		return true
	}

	return false
}

// RebuildGroups rebuilds the forwarder groups for all conditional forwarders.
// This should be called when forwarders are added/removed.
func (m *ConditionalForwarderManager) RebuildGroups(allForwarders []*Forwarder) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build a map of forwarder ID -> Forwarder.
	fwdMap := make(map[string]*Forwarder)
	for _, f := range allForwarders {
		fwdMap[f.ID] = f
	}

	for _, cf := range m.conditionals {
		var fwdList []*Forwarder
		for _, id := range cf.ForwarderIDs {
			if f, ok := fwdMap[id]; ok {
				fwdList = append(fwdList, f)
			}
		}

		cf.forwarders = fwdList
		cf.group = NewForwarderGroup(StrategySequential, 5*time.Second)
		cf.group.SetForwarders(fwdList)
	}
}

// SortedByDomainLength returns conditionals sorted by domain length (longest first).
func (m *ConditionalForwarderManager) SortedByDomainLength() []*ConditionalForwarder {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ConditionalForwarder, len(m.conditionals))
	copy(result, m.conditionals)
	sort.Slice(result, func(i, j int) bool {
		return len(result[i].Domain) > len(result[j].Domain)
	})
	return result
}
