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

// Package spec provides shared path-resolution and pattern-matching utilities
// used by the wink transformation operations.
package spec

import (
	"strings"
)

// WildcardKey is the special key that matches any map key.
const WildcardKey = "*"

// SetPath sets value at dotted path inside root, creating intermediate maps as needed.
// Example: SetPath(root, "a.b.c", 42) sets root["a"]["b"]["c"] = 42.
func SetPath(root map[string]any, path string, value any) {
	parts := strings.SplitN(path, ".", 2)
	key := parts[0]
	if len(parts) == 1 {
		// Leaf: merge or set.
		if existing, ok := root[key]; ok {
			root[key] = mergeValues(existing, value)
		} else {
			root[key] = value
		}
		return
	}
	child, ok := root[key]
	if !ok {
		child = map[string]any{}
		root[key] = child
	}
	childMap, ok := child.(map[string]any)
	if !ok {
		// Overwrite scalar with a map.
		childMap = map[string]any{}
		root[key] = childMap
	}
	SetPath(childMap, parts[1], value)
}

// mergeValues merges src into dst. When both are maps they are merged recursively.
// When dst is a slice, src is appended. Otherwise src replaces dst.
func mergeValues(dst, src any) any {
	dstMap, dstIsMap := dst.(map[string]any)
	srcMap, srcIsMap := src.(map[string]any)
	if dstIsMap && srcIsMap {
		for k, v := range srcMap {
			if existing, ok := dstMap[k]; ok {
				dstMap[k] = mergeValues(existing, v)
			} else {
				dstMap[k] = v
			}
		}
		return dstMap
	}
	dstSlice, dstIsList := dst.([]any)
	if dstIsList {
		return append(dstSlice, src)
	}
	return src
}

// GetPath returns the value at a dotted path, or (nil, false) if absent.
func GetPath(root map[string]any, path string) (any, bool) {
	parts := strings.SplitN(path, ".", 2)
	val, ok := root[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return val, true
	}
	child, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}
	return GetPath(child, parts[1])
}

// DeletePath removes the key at the given dotted path. Returns true if something was deleted.
func DeletePath(root map[string]any, path string) bool {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 1 {
		if _, ok := root[parts[0]]; ok {
			delete(root, parts[0])
			return true
		}
		return false
	}
	child, ok := root[parts[0]].(map[string]any)
	if !ok {
		return false
	}
	return DeletePath(child, parts[1])
}

// MatchingKeys returns input map keys that match the spec key.
// If specKey == "*", all keys are returned. Otherwise only specKey itself
// (if present) is returned.
func MatchingKeys(input map[string]any, specKey string) []string {
	if specKey == WildcardKey {
		keys := make([]string, 0, len(input))
		for k := range input {
			keys = append(keys, k)
		}
		return keys
	}
	if _, ok := input[specKey]; ok {
		return []string{specKey}
	}
	return nil
}
