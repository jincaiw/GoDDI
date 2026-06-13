package filter

import (
	"net"
	"sort"
	"sync"
)

// ClientPolicy represents a DNS policy applied to specific client IP ranges.
type ClientPolicy struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	SourceCIDR   string   `json:"source_cidr"`
	Action       string   `json:"action"` // allow, block, apply_lists
	BlockListIDs []string `json:"block_list_ids,omitempty"`
	AllowRuleIDs []string `json:"allow_rule_ids,omitempty"`
	Priority     int      `json:"priority"`
	Enabled      bool     `json:"enabled"`

	// Parsed CIDR for matching.
	cidr *net.IPNet
}

// ParseCIDR parses the SourceCIDR field.
func (p *ClientPolicy) ParseCIDR() error {
	_, ipNet, err := net.ParseCIDR(p.SourceCIDR)
	if err != nil {
		return err
	}
	p.cidr = ipNet
	return nil
}

// MatchesIP checks if a client IP matches this policy's CIDR.
func (p *ClientPolicy) MatchesIP(ip net.IP) bool {
	if p.cidr == nil {
		return false
	}
	return p.cidr.Contains(ip)
}

// ClientPolicyManager manages client-based policies.
type ClientPolicyManager struct {
	mu       sync.RWMutex
	policies []*ClientPolicy
}

// NewClientPolicyManager creates a new client policy manager.
func NewClientPolicyManager() *ClientPolicyManager {
	return &ClientPolicyManager{
		policies: make([]*ClientPolicy, 0),
	}
}

// AddPolicy adds a client policy.
func (m *ClientPolicyManager) AddPolicy(policy *ClientPolicy) error {
	if err := policy.ParseCIDR(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.policies = append(m.policies, policy)
	m.sortPolicies()
	return nil
}

// RemovePolicy removes a client policy by ID.
func (m *ClientPolicyManager) RemovePolicy(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, p := range m.policies {
		if p.ID == id {
			m.policies = append(m.policies[:i], m.policies[i+1:]...)
			return
		}
	}
}

// UpdatePolicy updates a client policy.
func (m *ClientPolicyManager) UpdatePolicy(policy *ClientPolicy) error {
	if err := policy.ParseCIDR(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, p := range m.policies {
		if p.ID == policy.ID {
			m.policies[i] = policy
			m.sortPolicies()
			return nil
		}
	}

	// Not found, add it.
	m.policies = append(m.policies, policy)
	m.sortPolicies()
	return nil
}

// GetPolicies returns all policies.
func (m *ClientPolicyManager) GetPolicies() []*ClientPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ClientPolicy, len(m.policies))
	copy(result, m.policies)
	return result
}

// MatchClientPolicy finds the best matching policy for a client IP.
// Returns the highest-priority matching policy, or nil if no match.
func (m *ClientPolicyManager) MatchClientPolicy(clientIP string) *ClientPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return nil
	}

	// Policies are sorted by priority (highest first).
	for _, p := range m.policies {
		if !p.Enabled {
			continue
		}
		if p.MatchesIP(ip) {
			return p
		}
	}

	return nil
}

// Reload clears and reloads all policies.
func (m *ClientPolicyManager) Reload(policies []*ClientPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.policies = make([]*ClientPolicy, 0, len(policies))
	for _, p := range policies {
		if err := p.ParseCIDR(); err == nil {
			m.policies = append(m.policies, p)
		}
	}
	m.sortPolicies()
}

// sortPolicies sorts policies by priority (highest first).
func (m *ClientPolicyManager) sortPolicies() {
	sort.Slice(m.policies, func(i, j int) bool {
		return m.policies[i].Priority > m.policies[j].Priority
	})
}
