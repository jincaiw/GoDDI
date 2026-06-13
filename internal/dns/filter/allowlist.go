package filter

import (
	"regexp"
	"sync"
)

// AllowListManager manages allow list (whitelist) rules.
// Allow rules override block list entries.
type AllowListManager struct {
	mu    sync.RWMutex
	trie  *DomainTrie
	rules map[string]MatchRule // ruleID -> rule
}

// NewAllowListManager creates a new allow list manager.
func NewAllowListManager() *AllowListManager {
	return &AllowListManager{
		trie:  NewDomainTrie(),
		rules: make(map[string]MatchRule),
	}
}

// AddRule adds an allow rule.
func (m *AllowListManager) AddRule(rule MatchRule) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Pre-compile regex patterns.
	if rule.MatchType == "regex" && rule.compiledRegex == nil {
		if re, err := regexp.Compile(rule.Pattern); err == nil {
			rule.compiledRegex = re
		}
	}

	m.rules[rule.ID] = rule
	if rule.Enabled {
		m.trie.Insert(rule)
	}
}

// RemoveRule removes an allow rule.
func (m *AllowListManager) RemoveRule(ruleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.rules, ruleID)
	m.trie.Remove(ruleID)
}

// GetRules returns all allow rules.
func (m *AllowListManager) GetRules() []MatchRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]MatchRule, 0, len(m.rules))
	for _, r := range m.rules {
		result = append(result, r)
	}
	return result
}

// CheckAllowList checks if a domain is in the allow list.
// Returns true if the domain should be allowed (whitelisted).
func (m *AllowListManager) CheckAllowList(qname string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, matched := m.trie.Match(qname)
	return matched
}

// Reload clears and reloads all allow list data.
func (m *AllowListManager) Reload(rules []MatchRule) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.trie = NewDomainTrie()
	m.rules = make(map[string]MatchRule)

	for i := range rules {
		r := &rules[i]
		// Pre-compile regex patterns.
		if r.MatchType == "regex" && r.compiledRegex == nil {
			if re, err := regexp.Compile(r.Pattern); err == nil {
				r.compiledRegex = re
			}
		}
		m.rules[r.ID] = *r
		if r.Enabled {
			m.trie.Insert(*r)
		}
	}
}

// RuleCount returns the total number of active allow rules.
func (m *AllowListManager) RuleCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trie.Size()
}
