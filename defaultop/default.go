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

// Package defaultop implements the Jolt "default" transform (Defaultr).
//
// Default walks the spec and asks "Does this key exist in the data?
// If not, add it."  Existing values are never overwritten.
//
// LHS wildcards supported:
//
//   - apply default to every key already present at this level
//     a|b        OR: apply to "a" or "b" if present
//     "key[]"    the value at "key" should be an array; spec children are
//     integer-keyed defaults for specific array indices
//
// Algorithm (matches Jolt):
//  1. Walk the spec depth-first.
//  2. At each level process literal keys first, then OR keys, then "*".
//  3. Literal keys force creation if absent; wildcards only touch keys that
//     already exist (either naturally or from an earlier literal default).
package defaultop

import (
	goSort "sort"
	"strings"

	"github.com/clowndaddy/wink/internal/pathutil"
	"github.com/clowndaddy/wink/internal/wildcards"
)

// Apply runs the default transform.
func Apply(input map[string]any, defaultSpec map[string]any) (map[string]any, error) {
	output := pathutil.DeepCopyMap(input)
	applyDefaults(output, defaultSpec)
	return output, nil
}

func applyDefaults(target map[string]any, spec map[string]any) {
	// Partition spec keys into literals, OR keys, and star wildcards.
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
	// OR keys sorted by number of alternatives then alphabetically.
	goSort.Slice(ors, func(i, j int) bool {
		ci := strings.Count(ors[i], "|")
		cj := strings.Count(ors[j], "|")
		if ci != cj {
			return ci < cj
		}
		return ors[i] < ors[j]
	})
	goSort.Strings(stars)

	// Process literals first: they CAN create new keys.
	for _, sk := range literals {
		applyDefault(target, sk, spec[sk], true)
	}
	// OR and * wildcards: only apply to already-present keys.
	for _, sk := range ors {
		applyDefault(target, sk, spec[sk], false)
	}
	for _, sk := range stars {
		applyDefault(target, sk, spec[sk], false)
	}
}

// applyDefault applies the default value to all matching keys.
// canCreate controls whether a missing key is created (true for literals only).
func applyDefault(target map[string]any, specKey string, specVal any, canCreate bool) {
	// Strip array marker from key.
	isArray := strings.HasSuffix(specKey, "[]")
	baseKey := strings.TrimSuffix(specKey, "[]")

	inputKeys := wildcards.MapKeys(target)
	matches := wildcards.MatchKeys(inputKeys, baseKey)

	// For literal keys not yet present, create them if canCreate.
	if len(matches) == 0 && canCreate && !strings.Contains(baseKey, "*") && !strings.Contains(baseKey, "|") {
		if isArray {
			target[baseKey] = []any{}
		} else {
			switch sv := specVal.(type) {
			case map[string]any:
				child := map[string]any{}
				applyDefaults(child, sv)
				target[baseKey] = child
			default:
				target[baseKey] = pathutil.DeepCopy(sv)
			}
		}
		return
	}

	for _, m := range matches {
		key := m.InputKey
		existing := target[key]

		if isArray {
			// Ensure the target value is a slice; then apply integer-keyed defaults.
			slice, ok := existing.([]any)
			if !ok {
				slice = []any{}
			}
			if childSpec, ok := specVal.(map[string]any); ok {
				for idxStr, idxVal := range childSpec {
					idx := atoi(idxStr)
					for len(slice) <= idx {
						slice = append(slice, nil)
					}
					if slice[idx] == nil {
						slice[idx] = pathutil.DeepCopy(idxVal)
					} else if childMap, ok := idxVal.(map[string]any); ok {
						if sliceMap, ok := slice[idx].(map[string]any); ok {
							applyDefaults(sliceMap, childMap)
						}
					}
				}
			}
			target[key] = slice
			continue
		}

		// Both spec and existing are maps → recurse.
		if childSpec, specIsMap := specVal.(map[string]any); specIsMap {
			if existing == nil {
				child := map[string]any{}
				applyDefaults(child, childSpec)
				target[key] = child
			} else if existingMap, ok := existing.(map[string]any); ok {
				applyDefaults(existingMap, childSpec)
			}
			// If existing is a non-map scalar, leave it alone.
			continue
		}

		// Spec value is a scalar: only set if key is absent or nil.
		if existing == nil {
			target[key] = pathutil.DeepCopy(specVal)
		}
	}
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
