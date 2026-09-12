package configver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// FieldChange is a single field-level difference between two revisions.
//
// Removed and added fields are reported as Old=nil / New=nil respectively
// rather than being omitted, so the caller can render "field removed" without
// having to compare key sets itself.
type FieldChange struct {
	Field string `json:"field"`
	Old   any    `json:"old"`
	New   any    `json:"new"`
}

// Diff computes the field-level difference between two revision contents.
//
// Comparison is at the top level of the JSON object. That is the right
// granularity for configuration: each key is one independent setting (a TTL, a
// pool bound, a policy), and a nested value that differs is far more useful
// reported as "this setting changed from X to Y" than exploded into leaf paths.
//
// The result is sorted by field name so repeated calls produce byte-identical
// output; a diff that reorders itself between requests is impossible to review.
func Diff(oldContent, newContent json.RawMessage) ([]FieldChange, error) {
	oldMap, err := decodeObject(oldContent)
	if err != nil {
		return nil, fmt.Errorf("decode old content: %w", err)
	}
	newMap, err := decodeObject(newContent)
	if err != nil {
		return nil, fmt.Errorf("decode new content: %w", err)
	}

	// Collect the union of keys so removals are reported too.
	keys := make(map[string]struct{}, len(oldMap)+len(newMap))
	for k := range oldMap {
		keys[k] = struct{}{}
	}
	for k := range newMap {
		keys[k] = struct{}{}
	}

	changes := make([]FieldChange, 0, len(keys))
	for k := range keys {
		oldVal, inOld := oldMap[k]
		newVal, inNew := newMap[k]

		if inOld && inNew && reflect.DeepEqual(oldVal, newVal) {
			continue
		}
		change := FieldChange{Field: k, Old: oldVal, New: newVal}
		if !inOld {
			change.Old = nil
		}
		if !inNew {
			change.New = nil
		}
		changes = append(changes, change)
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Field < changes[j].Field })
	return changes, nil
}

// decodeObject unmarshals content that must be a JSON object.
//
// A configuration snapshot is always an object; anything else (an array, a
// scalar, or invalid JSON) is a programming error in an adapter, and failing
// loudly here is better than silently reporting "no differences".
func decodeObject(content json.RawMessage) (map[string]any, error) {
	if len(content) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(content, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}
