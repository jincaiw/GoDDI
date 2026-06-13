package filter

import (
	"net"
	"regexp"
	"strings"
	"sync"
)

// TrieNode represents a node in the domain trie.
// The trie is built from domain labels in reverse order (from TLD down).
type TrieNode struct {
	children map[string]*TrieNode
	rules    []MatchRule
	isEnd    bool
}

// MatchRule represents a matching rule at a trie node.
type MatchRule struct {
	ID            string         `json:"id"`
	ListID        string         `json:"list_id"` // ID of the block list this rule belongs to
	Pattern       string         `json:"pattern"`
	MatchType     string         `json:"match_type"`    // exact, suffix, wildcard, regex
	ResponseType  string         `json:"response_type"` // NXDOMAIN, NODATA, REFUSED, CUSTOM_IP, DROP
	ResponseData  string         `json:"response_data"` // Custom IP or empty
	Enabled       bool           `json:"enabled"`
	compiledRegex *regexp.Regexp // pre-compiled regex for matchType == "regex"
}

// DomainTrie is a trie data structure for fast domain suffix matching.
type DomainTrie struct {
	root *TrieNode
	mu   sync.RWMutex
}

// NewDomainTrie creates a new domain trie.
func NewDomainTrie() *DomainTrie {
	return &DomainTrie{
		root: &TrieNode{
			children: make(map[string]*TrieNode),
		},
	}
}

// Insert adds a rule to the trie.
func (t *DomainTrie) Insert(rule MatchRule) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Pre-compile regex patterns.
	if rule.MatchType == "regex" && rule.compiledRegex == nil {
		if re, err := regexp.Compile(rule.Pattern); err == nil {
			rule.compiledRegex = re
		}
	}

	domain := strings.ToLower(strings.TrimSuffix(rule.Pattern, "."))
	labels := reverseLabels(domain)

	node := t.root
	for _, label := range labels {
		if node.children == nil {
			node.children = make(map[string]*TrieNode)
		}
		if _, ok := node.children[label]; !ok {
			node.children[label] = &TrieNode{
				children: make(map[string]*TrieNode),
			}
		}
		node = node.children[label]
	}
	node.isEnd = true
	node.rules = append(node.rules, rule)
}

// Remove removes a rule by ID from the trie.
func (t *DomainTrie) Remove(ruleID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// We need to traverse all nodes to find and remove the rule.
	t.removeRuleFromNode(t.root, ruleID, "")
}

func (t *DomainTrie) removeRuleFromNode(node *TrieNode, ruleID, parentLabel string) bool {
	for i, r := range node.rules {
		if r.ID == ruleID {
			node.rules = append(node.rules[:i], node.rules[i+1:]...)
			if len(node.rules) == 0 {
				node.isEnd = false
			}
			// Return true to indicate this node's rule was removed
			return len(node.rules) == 0 && !node.isEnd && len(node.children) == 0
		}
	}
	// Track children to delete after iteration to avoid concurrent map modification
	var emptyChildLabels []string
	for label, child := range node.children {
		if t.removeRuleFromNode(child, ruleID, label) {
			emptyChildLabels = append(emptyChildLabels, label)
		}
	}
	// Delete empty children
	for _, label := range emptyChildLabels {
		delete(node.children, label)
	}
	// This node can be deleted if it has no rules, is not an endpoint, and has no children
	return len(node.rules) == 0 && !node.isEnd && len(node.children) == 0
}

// Match checks if a domain matches any rule in the trie.
// Returns the matched rule and true if found, nil and false otherwise.
// It performs longest suffix matching.
func (t *DomainTrie) Match(domain string) (*MatchRule, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	labels := reverseLabels(domain)

	var bestMatch *MatchRule
	node := t.root

	for i, label := range labels {
		child, ok := node.children[label]
		if !ok {
			// Check for wildcard.
			child, ok = node.children["*"]
			if !ok {
				break
			}
		}

		node = child

		if node.isEnd {
			// Check if this is an exact or suffix match.
			for j := range node.rules {
				r := &node.rules[j]
				if !r.Enabled {
					continue
				}
				if r.MatchType == "exact" {
					// For exact match, the number of labels must match.
					if i == len(labels)-1 {
						if bestMatch == nil || len(r.Pattern) > len(bestMatch.Pattern) {
							bestMatch = r
						}
					}
				} else {
					// suffix or wildcard match.
					if bestMatch == nil || len(r.Pattern) > len(bestMatch.Pattern) {
						bestMatch = r
					}
				}
			}
		}
	}

	return bestMatch, bestMatch != nil
}

// MatchAll returns all rules that match the given domain.
func (t *DomainTrie) MatchAll(domain string) []MatchRule {
	t.mu.RLock()
	defer t.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	labels := reverseLabels(domain)

	var matches []MatchRule
	node := t.root

	for i, label := range labels {
		child, ok := node.children[label]
		if !ok {
			child, ok = node.children["*"]
			if !ok {
				break
			}
		}

		node = child

		if node.isEnd {
			for j := range node.rules {
				r := node.rules[j]
				if !r.Enabled {
					continue
				}
				if r.MatchType == "exact" && i != len(labels)-1 {
					continue
				}
				matches = append(matches, r)
			}
		}
	}

	return matches
}

// Size returns the number of rules in the trie.
func (t *DomainTrie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.countRules(t.root)
}

func (t *DomainTrie) countRules(node *TrieNode) int {
	count := len(node.rules)
	for _, child := range node.children {
		count += t.countRules(child)
	}
	return count
}

// reverseLabels splits a domain into labels and reverses them.
// e.g., "www.example.com" -> ["com", "example", "www"]
func reverseLabels(domain string) []string {
	if domain == "" || domain == "." {
		return []string{}
	}
	labels := strings.Split(domain, ".")
	// Reverse.
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
	return labels
}

// privateNets holds pre-parsed private IP networks, initialized once.
var privateNets []*net.IPNet

func init() {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"fc00::/7",
		"::1/128",
		"fe80::/10",
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		privateNets = append(privateNets, network)
	}
}

// IsPrivateIP checks if an IP address is in a private range.
func IsPrivateIP(ip net.IP) bool {
	for _, network := range privateNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
