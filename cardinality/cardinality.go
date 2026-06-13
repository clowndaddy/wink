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

// Package cardinality implements the Jolt "cardinality" transform.
//
// The cardinality transform normalises the shape of input fields so that
// downstream transforms can rely on consistent types.
//
// Valid cardinality values (RHS): "ONE" or "MANY".
// The "*" wildcard on the LHS iterates over all keys at that level, or over
// all elements if the matched value is an array.
package cardinality

import (
	goSort "sort"
	"strings"

	"wink/internal/pathutil"
	"wink/internal/wildcards"
)

// Apply runs the cardinality transform.
func Apply(input map[string]any, cardSpec map[string]any) (map[string]any, error) {
	output := pathutil.DeepCopyMap(input)
	applyCard(output, cardSpec)
	return output, nil
}

func applyCard(target map[string]any, spec map[string]any) {
	inputKeys := wildcards.MapKeys(target)

	// Partition into literals, OR, and star wildcards — process in that order.
	var literals, ors, stars []string
	for sk := range spec {
		switch {
		case strings.Contains(sk, "|"):
			ors = append(ors, sk)
		case sk == "*":
			stars = append(stars, sk)
		default:
			literals = append(literals, sk)
		}
	}
	goSort.Strings(literals)
	goSort.Strings(ors)
	goSort.Strings(stars)

	process := func(sk string) {
		sv := spec[sk]
		matches := wildcards.MatchKeys(inputKeys, sk)
		for _, m := range matches {
			key := m.InputKey
			applyToKey(target, key, sv)
		}
	}

	for _, sk := range literals {
		process(sk)
	}
	for _, sk := range ors {
		process(sk)
	}
	for _, sk := range stars {
		process(sk)
	}
}

// applyToKey applies the spec value to target[key].
func applyToKey(target map[string]any, key string, sv any) {
	switch sv := sv.(type) {
	case string:
		// Leaf: "ONE" or "MANY" applied directly to this key's value.
		mode := strings.ToUpper(sv)
		switch mode {
		case "ONE":
			target[key] = toOne(target[key])
		case "MANY":
			target[key] = toMany(target[key])
		}

	case map[string]any:
		// Nested spec.
		// Handle "@": "ONE"/"MANY" — apply cardinality to the value at THIS key,
		// then recurse into the (possibly collapsed) value with remaining sub-keys.
		nestedSpec := map[string]any{}
		for nk, nv := range sv {
			if nk == "@" {
				mode := strings.ToUpper(nv.(string))
				switch mode {
				case "ONE":
					target[key] = toOne(target[key])
				case "MANY":
					target[key] = toMany(target[key])
				}
			} else {
				nestedSpec[nk] = nv
			}
		}

		if len(nestedSpec) == 0 {
			return
		}

		// Recurse into the (now possibly modified) value.
		switch child := target[key].(type) {
		case map[string]any:
			// Child is a map: apply the nested spec to it directly.
			applyCard(child, nestedSpec)

		case []any:
			// Child is an array.
			// If the nested spec is {"*": subSpec}, the "*" means "for each
			// element in the array" — apply subSpec directly to each element map.
			// Otherwise apply nestedSpec directly to each element map.
			effectiveSpec := nestedSpec
			if len(nestedSpec) == 1 {
				if starVal, hasStar := nestedSpec["*"]; hasStar {
					if starMap, ok := starVal.(map[string]any); ok {
						effectiveSpec = starMap
					}
				}
			}
			for _, elem := range child {
				if elemMap, ok := elem.(map[string]any); ok {
					applyCard(elemMap, effectiveSpec)
				}
			}
		}
	}
}

// toOne converts a list to its first element; all other types are no-ops.
func toOne(v any) any {
	if list, ok := v.([]any); ok {
		if len(list) == 0 {
			return nil
		}
		return list[0]
	}
	return v
}

// toMany wraps a non-list in a single-element list; null → []; list → no-op.
func toMany(v any) any {
	if _, ok := v.([]any); ok {
		return v
	}
	if v == nil {
		return []any{}
	}
	return []any{v}
}
