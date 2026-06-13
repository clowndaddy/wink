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

// Package shift implements the Jolt "shift" transform.
//
// Shift copies data from the input JSON tree to the output JSON tree.
// Any input key not referenced by the spec disappears from the output.
//
// LHS wildcards:
//   - literal      exact key match
//   - a|b          OR: match either
//   - prefix-*     glob; captures starred portion
//   - *            match all keys (non-greedy, last priority)
//   - &, &N, &(N,M) dereference path key
//   - $            use matched input KEY as the output value
//   - #literal      write literal string as output value
//   - @            copy input value at current level to output
//   - @(N,key)     look up value N levels up under key
//   - \X           escape: treat X as literal
//
// RHS path syntax:
//   - dot.notation   nested map
//   - key[N]         specific array index
//   - key[]          append to array
//   - [#N]           use match count N levels up as array index
//   - &, &N, &(N,M)  reference a matched key from the walk path
package shift

import (
	goSort "sort"
	"strings"

	"github.com/clowndaddy/wink/internal/pathutil"
	"github.com/clowndaddy/wink/internal/wildcards"
)

// Apply runs the shift transform.
func Apply(input any, shiftSpec map[string]any) (map[string]any, error) {
	output := map[string]any{}
	walk(input, shiftSpec, output, nil, []int{})
	return output, nil
}

// walk recursively processes the spec against input, writing results into output.
// path[0] = innermost (current) PathEntry; path[1] = one level up, etc.
// matchCounts[0] = matches at current level (for [#N] RHS syntax).
func walk(input any, spec map[string]any, output map[string]any, path []wildcards.PathEntry, matchCounts []int) {
	inputMap := toMap(input)

	// --- Unconditional special LHS keys: $, #, @ ---
	// These fire based on the PARENT match, not on input key matching.
	// Process them before any key-based matching.
	for sk, sv := range spec {
		stripped := stripEscape(sk)
		svStr, isStr := sv.(string)

		if strings.HasPrefix(stripped, "#") && isStr {
			// #literal: write the literal string (the part after #) to the RHS path.
			literal := stripped[1:]
			dest := resolveRHS(svStr, path, matchCounts)
			if dest != "" {
				pathutil.Set(output, dest, literal)
			}
			continue
		}

		if strings.HasPrefix(stripped, "$") && isStr {
			// $: use the matched input KEY (from path) as the output value.
			_, _, level, sub := parseSpecialToken(stripped, '$')
			keyVal := wildcards.ResolveAmpersand(buildAmpRef(level, sub), path)
			dest := resolveRHS(svStr, path, matchCounts)
			if dest != "" {
				pathutil.Set(output, dest, keyVal)
			}
			continue
		}

		if (stripped == "@" || strings.HasPrefix(stripped, "@(")) && isStr {
			// @: copy input value at current level (or looked-up ancestor value).
			var val any
			if stripped == "@" {
				val = input
			} else {
				val = resolveAtLookup(stripped, input, path)
			}
			dest := resolveRHS(svStr, path, matchCounts)
			if dest != "" {
				pathutil.Set(output, dest, val)
			}
			continue
		}
	}

	// If there's no map to iterate over, we're done after the unconditional keys.
	if inputMap == nil {
		return
	}
	inputKeys := wildcards.MapKeys(inputMap)

	// Partition remaining spec keys into priority buckets.
	// Literals (including escaped and OR) → & computed → * wildcards.
	var literals, ampersands, stars []string
	for sk := range spec {
		stripped := stripEscape(sk)
		// Skip unconditional special keys — already handled above.
		if strings.HasPrefix(stripped, "$") || strings.HasPrefix(stripped, "#") ||
			stripped == "@" || strings.HasPrefix(stripped, "@(") {
			continue
		}
		switch {
		case strings.HasPrefix(sk, `\`):
			literals = append(literals, sk)
		case strings.Contains(sk, "|"):
			literals = append(literals, sk)
		case strings.Contains(sk, "&"):
			ampersands = append(ampersands, sk)
		case strings.Contains(sk, "*"):
			stars = append(stars, sk)
		default:
			literals = append(literals, sk)
		}
	}
	goSort.Strings(literals)
	goSort.Strings(ampersands)
	goSort.Strings(stars)

	// Track which input keys have been claimed by a more-specific rule.
	claimed := map[string]bool{}

	processKey := func(sk string, allowClaim bool) {
		sv := spec[sk]
		matches := wildcards.MatchKeys(inputKeys, sk)

		for _, m := range matches {
			if claimed[m.InputKey] {
				continue
			}
			if allowClaim && !strings.Contains(sk, "*") {
				claimed[m.InputKey] = true
			}

			ik := m.InputKey
			inputVal := inputMap[ik]
			entry := wildcards.PathEntry{Key: ik, Stars: m.Stars}
			newPath := prependEntry(entry, path)
			newCounts := prependCount(0, matchCounts)

			switch sv := sv.(type) {
			case string:
				dest := resolveRHS(sv, newPath, newCounts)
				if dest != "" {
					pathutil.Set(output, dest, inputVal)
				}
			case []any:
				for _, d := range sv {
					if ds, ok := d.(string); ok {
						dest := resolveRHS(ds, newPath, newCounts)
						if dest != "" {
							pathutil.Set(output, dest, inputVal)
						}
					}
				}
			case map[string]any:
				walk(inputVal, sv, output, newPath, newCounts)
			}
		}
	}

	for _, sk := range literals {
		processKey(sk, true)
	}
	for _, sk := range ampersands {
		processKey(sk, true)
	}
	for _, sk := range stars {
		processKey(sk, false)
	}
}

// resolveRHS resolves & and [#N] tokens in an RHS path string.
func resolveRHS(rhs string, path []wildcards.PathEntry, matchCounts []int) string {
	rhs = wildcards.ResolveAmpersand(rhs, path)
	rhs = wildcards.ResolveHashIndex(rhs, matchCounts)
	return rhs
}

// resolveAtLookup resolves "@(N,key)" by looking up key in the context map.
func resolveAtLookup(spec string, currentInput any, _ []wildcards.PathEntry) any {
	// Parse @(N,key) — for now resolve from currentInput map.
	inner := spec[2 : len(spec)-1]
	parts := strings.SplitN(inner, ",", 2)
	if len(parts) != 2 {
		return currentInput
	}
	key := strings.TrimSpace(parts[1])
	if m, ok := currentInput.(map[string]any); ok {
		return m[key]
	}
	return nil
}

// toMap coerces input to map[string]any, treating arrays as integer-keyed maps.
func toMap(input any) map[string]any {
	switch v := input.(type) {
	case map[string]any:
		return v
	case []any:
		m := make(map[string]any, len(v))
		for i, elem := range v {
			m[itoa(i)] = elem
		}
		return m
	}
	return nil
}

func stripEscape(s string) string {
	if strings.HasPrefix(s, `\`) {
		return s[1:]
	}
	return s
}

func prependEntry(e wildcards.PathEntry, path []wildcards.PathEntry) []wildcards.PathEntry {
	out := make([]wildcards.PathEntry, len(path)+1)
	out[0] = e
	copy(out[1:], path)
	return out
}

func prependCount(n int, counts []int) []int {
	out := make([]int, len(counts)+1)
	out[0] = n
	copy(out[1:], counts)
	return out
}

func buildAmpRef(level, sub int) string {
	if sub == 0 {
		return "&(" + itoa(level) + ")"
	}
	return "&(" + itoa(level) + "," + itoa(sub) + ")"
}

func parseSpecialToken(s string, prefix byte) (tok string, advance, level, sub int) {
	if len(s) == 0 || s[0] != prefix {
		return "", 0, 0, 0
	}
	if len(s) == 1 {
		return string(prefix), 1, 0, 0
	}
	if s[1] == '(' {
		j := strings.Index(s[1:], ")")
		if j < 0 {
			return s, len(s), 0, 0
		}
		inner := s[2 : 1+j]
		parts := strings.SplitN(inner, ",", 2)
		level = atoi(strings.TrimSpace(parts[0]))
		if len(parts) == 2 {
			sub = atoi(strings.TrimSpace(parts[1]))
		}
		end := 1 + j + 1
		return s[:end], end, level, sub
	}
	start := 1
	i := start
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return string(prefix), 1, 0, 0
	}
	level = atoi(s[start:i])
	return s[:i], i, level, 0
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
