package filter

import (
	"log/slog"
	"regexp"
	"sync"
)

// BlockList represents a named collection of blocking rules.
type BlockList struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"` // custom, external
	URL        string `json:"url,omitempty"`
	Enabled    bool   `json:"enabled"`
	EntryCount int    `json:"entry_count"`
}

// BlockListManager manages block lists and their rules.
type BlockListManager struct {
	mu    sync.RWMutex
	trie  *DomainTrie
	lists map[string]*BlockList
	rules map[string][]MatchRule // listID -> rules
}

// NewBlockListManager creates a new block list manager.
func NewBlockListManager() *BlockListManager {
	return &BlockListManager{
		trie:  NewDomainTrie(),
		lists: make(map[string]*BlockList),
		rules: make(map[string][]MatchRule),
	}
}

// AddList adds a block list.
func (m *BlockListManager) AddList(list *BlockList) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists[list.ID] = list
}

// RemoveList removes a block list and all its rules.
func (m *BlockListManager) RemoveList(listID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, ok := m.rules[listID]
	if ok {
		for _, r := range rules {
			m.trie.Remove(r.ID)
		}
		delete(m.rules, listID)
	}
	delete(m.lists, listID)
}

// UpdateList updates a block list metadata.
func (m *BlockListManager) UpdateList(list *BlockList) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists[list.ID] = list
}

// GetList returns a block list by ID.
func (m *BlockListManager) GetList(listID string) (*BlockList, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list, ok := m.lists[listID]
	return list, ok
}

// ListLists returns all block lists.
func (m *BlockListManager) ListLists() []*BlockList {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*BlockList, 0, len(m.lists))
	for _, list := range m.lists {
		result = append(result, list)
	}
	return result
}

// AddRule adds a rule to a block list.
func (m *BlockListManager) AddRule(listID string, rule MatchRule) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Pre-compile regex patterns.
	if rule.MatchType == "regex" && rule.compiledRegex == nil {
		if re, err := regexp.Compile(rule.Pattern); err == nil {
			rule.compiledRegex = re
		}
	}

	m.rules[listID] = append(m.rules[listID], rule)
	m.trie.Insert(rule)

	// Update entry count.
	if list, ok := m.lists[listID]; ok {
		list.EntryCount = len(m.rules[listID])
	}
}

// RemoveRule removes a rule from a block list.
func (m *BlockListManager) RemoveRule(listID, ruleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, ok := m.rules[listID]
	if !ok {
		return
	}

	for i, r := range rules {
		if r.ID == ruleID {
			m.rules[listID] = append(rules[:i], rules[i+1:]...)
			m.trie.Remove(ruleID)
			break
		}
	}

	// Update entry count.
	if list, ok := m.lists[listID]; ok {
		list.EntryCount = len(m.rules[listID])
	}
}

// GetRules returns all rules for a block list.
func (m *BlockListManager) GetRules(listID string) []MatchRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rules, ok := m.rules[listID]
	if !ok {
		return nil
	}
	result := make([]MatchRule, len(rules))
	copy(result, rules)
	return result
}

// CheckBlockList checks if a domain should be blocked.
// Returns (blocked bool, responseType string, responseData string).
func (m *BlockListManager) CheckBlockList(qname string, clientIP string) (bool, string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, matched := m.trie.Match(qname)
	if !matched {
		return false, "", ""
	}

	// Check if the list is enabled.
	if list, ok := m.lists[rule.ListID]; ok {
		if !list.Enabled {
			return false, "", ""
		}
	}

	slog.Debug("blocklist: domain blocked",
		"domain", qname,
		"rule_id", rule.ID,
		"pattern", rule.Pattern,
		"response_type", rule.ResponseType,
	)

	return true, rule.ResponseType, rule.ResponseData
}

// Reload clears and reloads all block list data.
func (m *BlockListManager) Reload(lists []*BlockList, rulesMap map[string][]MatchRule) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Rebuild trie.
	m.trie = NewDomainTrie()
	m.lists = make(map[string]*BlockList)
	m.rules = make(map[string][]MatchRule)

	for _, list := range lists {
		m.lists[list.ID] = list
	}

	for listID, rules := range rulesMap {
		m.rules[listID] = rules
		for i := range rules {
			rules[i].ListID = listID // Set ListID so CheckBlockList can look up the list
			// Pre-compile regex patterns.
			if rules[i].MatchType == "regex" && rules[i].compiledRegex == nil {
				if re, err := regexp.Compile(rules[i].Pattern); err == nil {
					rules[i].compiledRegex = re
				}
			}
			if rules[i].Enabled {
				m.trie.Insert(rules[i])
			}
		}
	}

	totalRules := m.trie.Size()
	slog.Info("blocklist: reloaded", "lists", len(lists), "rules", totalRules)
}

// RuleCount returns the total number of active rules.
func (m *BlockListManager) RuleCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trie.Size()
}
