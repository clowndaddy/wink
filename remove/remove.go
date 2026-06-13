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

// Package remove implements the Jolt "remove" transform (Removr).
//
// Remove walks the spec and asks "If this exists in the data, remove it."
//
// Spec format:
//
//	{ "keyToRemove": "" }              removes keyToRemove from root
//	{ "parent": { "child": "" } }      removes child from parent
//	{ "*": "" }                        removes ALL keys at this level
//	{ "tag-*": "" }                    removes all keys matching the glob
//	{ "a|b": "" }                      removes "a" and/or "b"
//	{ "array": { "0": "" } }           removes index 0 from array "array"
//
// The spec value for a leaf removal is always "".
// A nested map value causes recursion into the child.
package remove

import (
	goSort "sort"
	"strconv"
	"strings"

	"wink/internal/pathutil"
	"wink/internal/wildcards"
)

// Apply runs the remove transform.
func Apply(input map[string]any, removeSpec map[string]any) (map[string]any, error) {
	output := pathutil.DeepCopyMap(input)
	applyRemove(output, removeSpec)
	return output, nil
}

func applyRemove(target map[string]any, spec map[string]any) {
	inputKeys := wildcards.MapKeys(target)

	for specKey, specVal := range spec {
		matches := wildcards.MatchKeys(inputKeys, specKey)

		for _, m := range matches {
			key := m.InputKey

			switch sv := specVal.(type) {
			case string:
				// "" means delete this key.
				delete(target, key)

			case map[string]any:
				if len(sv) == 0 {
					delete(target, key)
					continue
				}
				// Recurse into child.
				switch child := target[key].(type) {
				case map[string]any:
					applyRemove(child, sv)
				case []any:
					applyRemoveFromArray(child, sv, target, key)
				}
			}
		}
	}
}

// applyRemoveFromArray removes elements from a JSON array.
// The spec keys are expected to be numeric indices (as strings).
// Per Jolt spec: indices are removed from largest to smallest to avoid
// offset issues.
func applyRemoveFromArray(arr []any, spec map[string]any, parent map[string]any, parentKey string) {
	// Collect wildcard-matched and literal index specs.
	var indicesToRemove []int
	var subSpecs []struct {
		idx  int
		spec map[string]any
	}

	inputKeys := make([]string, len(arr))
	for i := range arr {
		inputKeys[i] = strconv.Itoa(i)
	}

	for specKey, specVal := range spec {
		matches := wildcards.MatchKeys(inputKeys, specKey)
		for _, m := range matches {
			idx := atoi(m.InputKey)
			switch sv := specVal.(type) {
			case string:
				indicesToRemove = append(indicesToRemove, idx)
			case map[string]any:
				if len(sv) == 0 {
					indicesToRemove = append(indicesToRemove, idx)
				} else {
					// Recurse.
					subSpecs = append(subSpecs, struct {
						idx  int
						spec map[string]any
					}{idx, sv})
				}
			}
		}
	}

	// Handle sub-spec recursion first.
	for _, ss := range subSpecs {
		if ss.idx < len(arr) {
			if childMap, ok := arr[ss.idx].(map[string]any); ok {
				applyRemove(childMap, ss.spec)
			}
		}
	}

	// Remove indices largest-first.
	goSort.Sort(goSort.Reverse(goSort.IntSlice(indicesToRemove)))
	for _, idx := range indicesToRemove {
		if idx < len(arr) {
			arr = append(arr[:idx], arr[idx+1:]...)
		}
	}
	parent[parentKey] = arr
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// Ensure strings import is used.
var _ = strings.Contains
