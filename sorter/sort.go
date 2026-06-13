// Copyright 2024 John Stamper
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package sorter implements the Jolt "sort" transform.
//
// Sort recursively sorts all map keys alphabetically throughout the JSON tree.
// The sort order is standard ascending alphabetical, with the special case that
// keys prefixed with "~" sort to the TOP (Jolt's documented behaviour).
package sorter

import (
	gojson "encoding/json"
	goSort "sort"
	"strings"
)

// SortedMap is an exported map type whose MarshalJSON emits keys in Jolt sort
// order: "~"-prefixed keys first, then all others alphabetically.
// wink.go uses the concrete type to preserve ordering through TransformJSON.
type SortedMap map[string]any

// Apply runs the sort transform and returns a SortedMap so that json.Marshal
// produces correctly ordered output.
func Apply(input map[string]any) (SortedMap, error) {
	return sortMap(input), nil
}

func sortMap(m map[string]any) SortedMap {
	out := make(SortedMap, len(m))
	for k, v := range m {
		out[k] = sortValue(v)
	}
	return out
}

func sortValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return sortMap(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = sortValue(item)
		}
		return out
	default:
		return v
	}
}

// MarshalJSON emits keys in Jolt sort order:
//   - "~"-prefixed keys come first (alphabetically among themselves)
//   - all other keys follow, alphabetically
func (sm SortedMap) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(sm))
	for k := range sm {
		keys = append(keys, k)
	}
	goSort.Slice(keys, func(i, j int) bool {
		ki, kj := keys[i], keys[j]
		iTilde := strings.HasPrefix(ki, "~")
		jTilde := strings.HasPrefix(kj, "~")
		if iTilde != jTilde {
			return iTilde
		}
		return ki < kj
	})

	buf := []byte{'{'}
	for i, k := range keys {
		kb, err := gojson.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := gojson.Marshal(sm[k])
		if err != nil {
			return nil, err
		}
		buf = append(buf, kb...)
		buf = append(buf, ':')
		buf = append(buf, vb...)
		if i < len(keys)-1 {
			buf = append(buf, ',')
		}
	}
	buf = append(buf, '}')
	return buf, nil
}
