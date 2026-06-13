package filter

import (
	"testing"
)

func TestNewBlockListManager(t *testing.T) {
	t.Parallel()

	m := NewBlockListManager()
	if m == nil {
		t.Fatal("NewBlockListManager() returned nil")
	}
	if m.RuleCount() != 0 {
		t.Errorf("new manager should have 0 rules, got %d", m.RuleCount())
	}
}

func TestBlockListManager_AddList(t *testing.T) {
	t.Parallel()

	m := NewBlockListManager()
	list := &BlockList{
		ID:      "list-1",
		Name:    "Test Block List",
		Type:    "custom",
		Enabled: true,
	}
	m.AddList(list)

	got, ok := m.GetList("list-1")
	if !ok {
		t.Fatal("GetList() should return true for existing list")
	}
	if got.Name != "Test Block List" {
		t.Errorf("list name = %q, want %q", got.Name, "Test Block List")
	}
}

func TestBlockListManager_RemoveList(t *testing.T) {
	m := NewBlockListManager()

	list := &BlockList{ID: "list-1", Name: "Test", Enabled: true}
	m.AddList(list)

	// Add a rule to the list
	rule := MatchRule{
		ID:           "rule-1",
		Pattern:      "ads.example.com",
		MatchType:    "exact",
		ResponseType: "NXDOMAIN",
		Enabled:      true,
	}
	m.AddRule("list-1", rule)

	m.RemoveList("list-1")

	_, ok := m.GetList("list-1")
	if ok {
		t.Error("GetList() should return false after RemoveList()")
	}

	// Rule should also be removed
	if m.RuleCount() != 0 {
		t.Errorf("RuleCount() = %d after removing list, want 0", m.RuleCount())
	}
}

func TestBlockListManager_AddAndCheckRule(t *testing.T) {
	m := NewBlockListManager()

	list := &BlockList{ID: "list-1", Name: "Test", Enabled: true}
	m.AddList(list)

	tests := []struct {
		name        string
		rule        MatchRule
		testDomain  string
		wantBlocked bool
	}{
		{
			"exact match",
			MatchRule{ID: "r1", Pattern: "ads.example.com", MatchType: "exact", ResponseType: "NXDOMAIN", Enabled: true},
			"ads.example.com",
			true,
		},
		{
			"exact no match",
			MatchRule{ID: "r2", Pattern: "tracker.example.com", MatchType: "exact", ResponseType: "NXDOMAIN", Enabled: true},
			"other.example.com",
			false,
		},
		{
			"suffix match",
			MatchRule{ID: "r3", Pattern: "ads.example.com", MatchType: "suffix", ResponseType: "REFUSED", Enabled: true},
			"sub.ads.example.com",
			true,
		},
		{
			"wildcard match",
			MatchRule{ID: "r4", Pattern: "*.ads.example.com", MatchType: "wildcard", ResponseType: "NXDOMAIN", Enabled: true},
			"sub.ads.example.com",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m2 := NewBlockListManager()
			list2 := &BlockList{ID: "list-2", Name: "Test2", Enabled: true}
			m2.AddList(list2)
			m2.AddRule("list-2", tt.rule)

			blocked, _, _ := m2.CheckBlockList(tt.testDomain, "")
			if blocked != tt.wantBlocked {
				t.Errorf("CheckBlockList(%q) blocked = %v, want %v", tt.testDomain, blocked, tt.wantBlocked)
			}
		})
	}
}

func TestBlockListManager_RemoveRule(t *testing.T) {
	m := NewBlockListManager()

	list := &BlockList{ID: "list-1", Name: "Test", Enabled: true}
	m.AddList(list)

	rule := MatchRule{
		ID:           "rule-1",
		Pattern:      "ads.example.com",
		MatchType:    "exact",
		ResponseType: "NXDOMAIN",
		Enabled:      true,
	}
	m.AddRule("list-1", rule)

	// Should be blocked
	blocked, _, _ := m.CheckBlockList("ads.example.com", "")
	if !blocked {
		t.Error("domain should be blocked before rule removal")
	}

	// Remove the rule
	m.RemoveRule("list-1", "rule-1")

	// Should no longer be blocked
	blocked, _, _ = m.CheckBlockList("ads.example.com", "")
	if blocked {
		t.Error("domain should not be blocked after rule removal")
	}
}

func TestBlockListManager_DisabledRule(t *testing.T) {
	m := NewBlockListManager()

	list := &BlockList{ID: "list-1", Name: "Test", Enabled: true}
	m.AddList(list)

	rule := MatchRule{
		ID:           "rule-1",
		Pattern:      "ads.example.com",
		MatchType:    "exact",
		ResponseType: "NXDOMAIN",
		Enabled:      false, // Disabled
	}
	m.AddRule("list-1", rule)

	// Disabled rules should not block
	blocked, _, _ := m.CheckBlockList("ads.example.com", "")
	if blocked {
		t.Error("disabled rule should not block")
	}
}

func TestBlockListManager_ListLists(t *testing.T) {
	m := NewBlockListManager()

	m.AddList(&BlockList{ID: "list-1", Name: "List 1", Enabled: true})
	m.AddList(&BlockList{ID: "list-2", Name: "List 2", Enabled: true})

	lists := m.ListLists()
	if len(lists) != 2 {
		t.Errorf("ListLists() returned %d lists, want 2", len(lists))
	}
}

func TestBlockListManager_GetRules(t *testing.T) {
	m := NewBlockListManager()

	list := &BlockList{ID: "list-1", Name: "Test", Enabled: true}
	m.AddList(list)

	m.AddRule("list-1", MatchRule{ID: "r1", Pattern: "a.com", MatchType: "exact", Enabled: true})
	m.AddRule("list-1", MatchRule{ID: "r2", Pattern: "b.com", MatchType: "exact", Enabled: true})

	rules := m.GetRules("list-1")
	if len(rules) != 2 {
		t.Errorf("GetRules() returned %d rules, want 2", len(rules))
	}
}

func TestBlockListManager_GetRules_NonexistentList(t *testing.T) {
	m := NewBlockListManager()

	rules := m.GetRules("nonexistent")
	if rules != nil {
		t.Errorf("GetRules() for nonexistent list should return nil, got %v", rules)
	}
}

func TestBlockListManager_Reload(t *testing.T) {
	m := NewBlockListManager()

	// Add initial data
	list1 := &BlockList{ID: "list-1", Name: "Old List", Enabled: true}
	m.AddList(list1)
	m.AddRule("list-1", MatchRule{ID: "r1", Pattern: "old.com", MatchType: "exact", Enabled: true})

	// Reload with new data
	newLists := []*BlockList{
		{ID: "list-2", Name: "New List", Enabled: true},
	}
	newRules := map[string][]MatchRule{
		"list-2": {
			{ID: "r2", Pattern: "new.com", MatchType: "exact", ResponseType: "NXDOMAIN", Enabled: true},
		},
	}
	m.Reload(newLists, newRules)

	// Old list should be gone
	_, ok := m.GetList("list-1")
	if ok {
		t.Error("old list should be gone after reload")
	}

	// New list should exist
	_, ok = m.GetList("list-2")
	if !ok {
		t.Error("new list should exist after reload")
	}

	// New rule should work
	blocked, _, _ := m.CheckBlockList("new.com", "")
	if !blocked {
		t.Error("new.com should be blocked after reload")
	}

	// Old rule should not work
	blocked, _, _ = m.CheckBlockList("old.com", "")
	if blocked {
		t.Error("old.com should not be blocked after reload")
	}
}

func TestBlockListManager_RuleCount(t *testing.T) {
	m := NewBlockListManager()

	list := &BlockList{ID: "list-1", Name: "Test", Enabled: true}
	m.AddList(list)

	m.AddRule("list-1", MatchRule{ID: "r1", Pattern: "a.com", MatchType: "exact", Enabled: true})
	m.AddRule("list-1", MatchRule{ID: "r2", Pattern: "b.com", MatchType: "exact", Enabled: true})
	m.AddRule("list-1", MatchRule{ID: "r3", Pattern: "c.com", MatchType: "exact", Enabled: false})

	// RuleCount counts all rules in the trie (including disabled ones, since AddRule inserts all)
	count := m.RuleCount()
	if count != 3 {
		t.Errorf("RuleCount() = %d, want 3", count)
	}
}

func TestDomainTrie_ExactMatch(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	trie.Insert(MatchRule{
		ID: "r1", Pattern: "ads.example.com", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	rule, matched := trie.Match("ads.example.com")
	if !matched {
		t.Error("should match exact domain")
	}
	if rule.ID != "r1" {
		t.Errorf("matched rule ID = %q, want %q", rule.ID, "r1")
	}
}

func TestDomainTrie_SuffixMatch(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	trie.Insert(MatchRule{
		ID: "r1", Pattern: "example.com", MatchType: "suffix",
		ResponseType: "REFUSED", Enabled: true,
	})

	// Subdomain should match
	rule, matched := trie.Match("sub.example.com")
	if !matched {
		t.Error("subdomain should match suffix rule")
	}
	if rule.ID != "r1" {
		t.Errorf("matched rule ID = %q, want %q", rule.ID, "r1")
	}

	// Exact domain should also match suffix
	rule, matched = trie.Match("example.com")
	if !matched {
		t.Error("exact domain should match suffix rule")
	}
}

func TestDomainTrie_WildcardMatch(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	trie.Insert(MatchRule{
		ID: "r1", Pattern: "*.example.com", MatchType: "wildcard",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	// Subdomain should match wildcard
	_, matched := trie.Match("sub.example.com")
	if !matched {
		t.Error("subdomain should match wildcard rule")
	}
}

func TestDomainTrie_NoMatch(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	trie.Insert(MatchRule{
		ID: "r1", Pattern: "ads.example.com", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	_, matched := trie.Match("other.example.com")
	if matched {
		t.Error("non-matching domain should not match")
	}
}

func TestDomainTrie_CaseInsensitive(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	trie.Insert(MatchRule{
		ID: "r1", Pattern: "ADS.EXAMPLE.COM", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	_, matched := trie.Match("ads.example.com")
	if !matched {
		t.Error("matching should be case-insensitive")
	}
}

func TestDomainTrie_Remove(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	trie.Insert(MatchRule{
		ID: "r1", Pattern: "ads.example.com", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	trie.Remove("r1")

	_, matched := trie.Match("ads.example.com")
	if matched {
		t.Error("domain should not match after rule removal")
	}
}

func TestDomainTrie_Size(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	if trie.Size() != 0 {
		t.Errorf("empty trie Size() = %d, want 0", trie.Size())
	}

	trie.Insert(MatchRule{ID: "r1", Pattern: "a.com", MatchType: "exact", Enabled: true})
	trie.Insert(MatchRule{ID: "r2", Pattern: "b.com", MatchType: "exact", Enabled: true})

	if trie.Size() != 2 {
		t.Errorf("trie Size() = %d, want 2", trie.Size())
	}
}

func TestDomainTrie_MatchAll(t *testing.T) {
	t.Parallel()

	trie := NewDomainTrie()
	trie.Insert(MatchRule{ID: "r1", Pattern: "example.com", MatchType: "suffix", ResponseType: "REFUSED", Enabled: true})
	trie.Insert(MatchRule{ID: "r2", Pattern: "sub.example.com", MatchType: "exact", ResponseType: "NXDOMAIN", Enabled: true})

	matches := trie.MatchAll("sub.example.com")
	if len(matches) != 2 {
		t.Errorf("MatchAll() returned %d matches, want 2", len(matches))
	}
}

func TestReverseLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"three labels", "www.example.com", []string{"com", "example", "www"}},
		{"two labels", "example.com", []string{"com", "example"}},
		{"single label", "localhost", []string{"localhost"}},
		{"empty string", "", []string{}},
		{"root domain", ".", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := reverseLabels(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("reverseLabels(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("reverseLabels(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}
