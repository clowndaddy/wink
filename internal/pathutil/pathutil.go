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

// Package pathutil provides shared utilities for writing values into a nested
// map/array tree using dot-notation + bracket-notation output paths, and for
// deep-copying JSON-compatible values.
package pathutil

import (
	"strconv"
	"strings"
)

// Set writes value at the dot-separated (with optional [N] / [] bracket) path
// inside root, creating intermediate maps/arrays as needed.
// If the destination already has a value, the two are coerced into a JSON
// array (Jolt's "implicit array" rule).
func Set(root map[string]any, path string, value any) {
	if path == "" {
		return
	}
	setInto(root, path, value)
}

func setInto(node map[string]any, path string, value any) {
	// Split off the first segment (up to the first '.' not inside brackets).
	seg, rest := splitPath(path)

	// Does this segment have a bracket suffix?
	key, arrayIdx, isArray := parseBracket(seg)

	if rest == "" {
		// Leaf.
		if isArray {
			writeArray(node, key, arrayIdx, value)
		} else {
			node[key] = mergeOrAppend(node[key], value)
		}
		return
	}

	// Intermediate node.
	if isArray {
		// Navigate into an array element.
		arr := ensureArray(node, key)
		if arrayIdx == -1 {
			arrayIdx = len(arr)
		}
		for len(arr) <= arrayIdx {
			arr = append(arr, nil)
		}
		child, ok := arr[arrayIdx].(map[string]any)
		if !ok {
			child = map[string]any{}
			arr[arrayIdx] = child
		}
		node[key] = arr
		setInto(child, rest, value)
	} else {
		child := ensureMap(node, key)
		setInto(child, rest, value)
	}
}

// splitPath splits "a.b.c" into ("a", "b.c"), handling brackets like "a[1].b".
func splitPath(path string) (seg, rest string) {
	depth := 0
	for i, ch := range path {
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
		case '.':
			if depth == 0 {
				return path[:i], path[i+1:]
			}
		}
	}
	return path, ""
}

// parseBracket parses "key[N]" or "key[]" returning (key, index, true).
// index == -1 means append ([]). Returns (seg, 0, false) if no bracket.
func parseBracket(seg string) (key string, index int, isArray bool) {
	open := strings.Index(seg, "[")
	if open < 0 {
		return seg, 0, false
	}
	close := strings.Index(seg[open:], "]")
	if close < 0 {
		return seg, 0, false
	}
	inner := seg[open+1 : open+close]
	key = seg[:open]
	if inner == "" {
		return key, -1, true
	}
	n, err := strconv.Atoi(inner)
	if err != nil {
		return seg, 0, false
	}
	return key, n, true
}

// writeArray writes value into node[key] as an array at the given index.
func writeArray(node map[string]any, key string, index int, value any) {
	arr := ensureArray(node, key)
	if index == -1 {
		arr = append(arr, value)
	} else {
		for len(arr) <= index {
			arr = append(arr, nil)
		}
		arr[index] = value
	}
	node[key] = arr
}

func ensureArray(node map[string]any, key string) []any {
	if v, ok := node[key]; ok {
		if arr, ok := v.([]any); ok {
			return arr
		}
	}
	return []any{}
}

func ensureMap(node map[string]any, key string) map[string]any {
	if v, ok := node[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	m := map[string]any{}
	node[key] = m
	return m
}

// mergeOrAppend implements Jolt's implicit array creation rule.
func mergeOrAppend(existing, incoming any) any {
	if existing == nil {
		return incoming
	}
	if list, ok := existing.([]any); ok {
		return append(list, incoming)
	}
	return []any{existing, incoming}
}

// DeepCopy returns a deep copy of any JSON-compatible value.
func DeepCopy(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = DeepCopy(vv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = DeepCopy(vv)
		}
		return out
	default:
		return v
	}
}

// DeepCopyMap deep-copies a map[string]any.
func DeepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = DeepCopy(v)
	}
	return out
}

// GetPath retrieves the value at a dot-notation path, returning (nil, false) if absent.
func GetPath(root map[string]any, path string) (any, bool) {
	if path == "" {
		return root, true
	}
	seg, rest := splitPath(path)
	key, arrayIdx, isArray := parseBracket(seg)

	val, ok := root[key]
	if !ok {
		return nil, false
	}

	if isArray {
		arr, ok := val.([]any)
		if !ok || arrayIdx < 0 || arrayIdx >= len(arr) {
			return nil, false
		}
		val = arr[arrayIdx]
	}

	if rest == "" {
		return val, true
	}
	childMap, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}
	return GetPath(childMap, rest)
}
