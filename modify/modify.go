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

package modify

import (
	"strings"

	"wink/internal/pathutil"
	"wink/internal/wildcards"
)

// Apply runs a modify transform with the given write mode.
// The spec mirrors the shape of the data; spec values are either:
//   - A nested map                           — recurse into child
//   - "=functionName(args...)"               — evaluate and write
//   - a literal (string, number, bool, null) — write directly
//   - "@"                                    — passthrough (keep existing value)
func Apply(input map[string]any, spec map[string]any, mode WriteMode) (map[string]any, error) {
	output := pathutil.DeepCopyMap(input)
	applyModify(output, spec, mode)
	return output, nil
}

func applyModify(target map[string]any, spec map[string]any, mode WriteMode) {
	for specKey, specVal := range spec {
		inputKeys := wildcards.MapKeys(target)
		matches := wildcards.MatchKeys(inputKeys, specKey)

		// Literal keys not yet present: create them so we can set defaults/values.
		if len(matches) == 0 && !strings.Contains(specKey, "*") && !strings.Contains(specKey, "|") {
			matches = []wildcards.MatchResult{{InputKey: specKey}}
		}

		for _, m := range matches {
			key := m.InputKey

			switch sv := specVal.(type) {
			case map[string]any:
				applyNestedSpec(target, key, sv, mode)

			case string:
				if sv == "@" {
					continue
				}
				applyValue(target, key, sv, mode)

			default:
				applyLiteral(target, key, specVal, mode)
			}
		}
	}
}

// applyNestedSpec applies a nested map spec to target[key].
// If target[key] is a map, recurse directly.
// If target[key] is an array, apply the spec to each element map.
// If target[key] is absent, create an empty map and recurse.
func applyNestedSpec(target map[string]any, key string, sv map[string]any, mode WriteMode) {
	child := target[key]

	switch c := child.(type) {
	case map[string]any:
		applyModify(c, sv, mode)

	case []any:
		// The spec descends into an array: apply sv to each element.
		// Check whether sv itself contains a "*" key — if so, that "*" means
		// "for each element in this array, apply the sub-spec".
		// Otherwise apply sv directly to each element map.
		if starSpec, hasStar := sv["*"]; hasStar && len(sv) == 1 {
			if starMap, ok := starSpec.(map[string]any); ok {
				for _, elem := range c {
					if elemMap, ok := elem.(map[string]any); ok {
						applyModify(elemMap, starMap, mode)
					}
				}
				return
			}
		}
		// No star: apply sv directly to each element map.
		for _, elem := range c {
			if elemMap, ok := elem.(map[string]any); ok {
				applyModify(elemMap, sv, mode)
			}
		}

	default:
		// Key absent or scalar: create a new map and recurse.
		newMap := map[string]any{}
		applyModify(newMap, sv, mode)
		if len(newMap) > 0 {
			target[key] = newMap
		}
	}
}

// applyValue evaluates a spec value string (literal or =function(...)) and
// writes to target[key] according to the write mode.
func applyValue(target map[string]any, key, expr string, mode WriteMode) {
	existing, exists := target[key]

	switch mode {
	case Define:
		if exists {
			return
		}
	case Default:
		if exists && existing != nil {
			return
		}
	case Overwrite:
		// Always write.
	}

	result, err := evalFunction(expr, target)
	if err != nil {
		return
	}
	target[key] = result
}

// applyLiteral writes a non-string literal value respecting the write mode.
func applyLiteral(target map[string]any, key string, val any, mode WriteMode) {
	existing, exists := target[key]
	switch mode {
	case Define:
		if exists {
			return
		}
	case Default:
		if exists && existing != nil {
			return
		}
	}
	target[key] = val
}
