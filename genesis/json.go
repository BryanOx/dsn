package genesis

import (
	"bytes"
	"encoding/json"
	"sort"
)

// CanonicalJSON marshals a GenesisDoc to deterministic JSON.
// Since Go's json.Marshal outputs struct fields in declaration order,
// and the GenesisDoc struct fields are in a consistent order, this produces
// deterministic output for the same input.
// For map fields, keys are sorted to ensure deterministic output.
func CanonicalJSON(doc *GenesisDoc) ([]byte, error) {
	// Use json.Marshal which is deterministic for struct fields
	// The struct field order in GenesisDoc is stable
	return json.Marshal(doc)
}

// CanonicalJSONIndent produces canonical JSON with indentation for debugging.
func CanonicalJSONIndent(doc *GenesisDoc) ([]byte, error) {
	return json.MarshalIndent(doc, "", "  ")
}

// SortMapKeys sorts map[string]interface{} keys recursively for deterministic JSON.
// This is a helper for more complex cases where we need to sort map keys.
func sortMapKeys(m map[string]interface{}) {
	// Get all keys and sort them
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// For each key, recursively sort if the value is a map
	for _, k := range keys {
		if v, ok := m[k].(map[string]interface{}); ok {
			sortMapKeys(v)
		}
	}
}

// canonicalizeMap converts a map with sorted keys into a deterministically
// ordered structure suitable for JSON marshaling.
func canonicalizeMap(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(data))

	// Sort keys
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build result with sorted keys and recursively canonicalized values
	for _, k := range keys {
		v := data[k]
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = canonicalizeMap(val)
		default:
			result[k] = val
		}
	}

	return result
}

// MarshalJSONIndent marshals the genesis doc with indentation for debugging/display.
func MarshalJSONIndent(doc *GenesisDoc) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
