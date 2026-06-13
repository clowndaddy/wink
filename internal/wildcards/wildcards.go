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

// Package wildcards implements Jolt's LHS key-matching rules and RHS reference resolution.
//
// Jolt matches input keys against spec keys in priority order:
//  1. Literal matches (exact string, no special characters)
//  2. OR (|) matches  – "rating|Rating" matches either key
//  3. Ampersand (&) computed-value matches
//  4. Star (*) wildcard matches – non-greedy, matched last
//
// Special LHS-only keys ($, #, @) are handled by the caller, not this package.
package wildcards

import (
	"strings"
)

// MatchResult records a single LHS key match.
type MatchResult struct {
	InputKey string   // the actual key from the input map
	SpecKey  string   // the spec key that matched
	Stars    []string // sub-captures from * glob segments (index 0 = first *, etc.)
}

// PathEntry records one level of the spec walk path, used for & and $ resolution.
type PathEntry struct {
	Key   string   // the actual input key that was matched at this level
	Stars []string // wildcard captures: Stars[0] = first * capture, etc.
}

// MatchKeys returns every input key that matches specKey, along with captured wildcard segments.
// OR keys are expanded; glob patterns are matched non-greedily.
func MatchKeys(inputKeys []string, specKey string) []MatchResult {
	// Escaped character: treat as literal (strip leading backslash).
	if strings.HasPrefix(specKey, `\`) {
		literal := specKey[1:]
		for _, k := range inputKeys {
			if k == literal {
				return []MatchResult{{InputKey: k, SpecKey: specKey}}
			}
		}
		return nil
	}

	// OR wildcard: "a|b|c" — split and match each alternative.
	if strings.Contains(specKey, "|") {
		var out []MatchResult
		seen := map[string]bool{}
		for _, alt := range strings.Split(specKey, "|") {
			alt = strings.TrimSpace(alt)
			for _, r := range MatchKeys(inputKeys, alt) {
				if !seen[r.InputKey] {
					seen[r.InputKey] = true
					out = append(out, MatchResult{InputKey: r.InputKey, SpecKey: specKey, Stars: r.Stars})
				}
			}
		}
		return out
	}

	// Glob wildcard: specKey contains one or more "*".
	if strings.Contains(specKey, "*") {
		return globMatch(inputKeys, specKey)
	}

	// Literal match.
	for _, k := range inputKeys {
		if k == specKey {
			return []MatchResult{{InputKey: k, SpecKey: specKey}}
		}
	}
	return nil
}

// globMatch handles specKeys that contain one or more "*" segments.
func globMatch(inputKeys []string, pattern string) []MatchResult {
	parts := strings.Split(pattern, "*")
	var out []MatchResult
	for _, k := range inputKeys {
		if caps, ok := matchGlob(k, parts); ok {
			out = append(out, MatchResult{InputKey: k, SpecKey: pattern, Stars: caps})
		}
	}
	return out
}

// matchGlob checks whether s matches the glob described by parts (segments between "*").
// Returns the captured substrings for each *.
func matchGlob(s string, parts []string) ([]string, bool) {
	if len(parts) == 1 {
		return nil, s == parts[0]
	}
	if !strings.HasPrefix(s, parts[0]) {
		return nil, false
	}
	remaining := s[len(parts[0]):]
	caps := make([]string, 0, len(parts)-1)
	for i := 1; i < len(parts); i++ {
		suffix := parts[i]
		if i == len(parts)-1 {
			if !strings.HasSuffix(remaining, suffix) {
				return nil, false
			}
			caps = append(caps, remaining[:len(remaining)-len(suffix)])
			remaining = ""
		} else {
			idx := strings.Index(remaining, suffix)
			if idx < 0 {
				return nil, false
			}
			caps = append(caps, remaining[:idx])
			remaining = remaining[idx+len(suffix):]
		}
	}
	if remaining != "" {
		return nil, false
	}
	return caps, true
}

// MapKeys returns the keys of a map in unspecified order.
func MapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ResolveAmpersand resolves all & tokens in an RHS path string.
// pathEntries[0] = innermost (current) match; pathEntries[1] = one level up, etc.
//
// Syntax:
//
//	&        = &(0,0)  current matched key
//	&N       = &(N,0)  key N levels up
//	&(N)     = &(N,0)
//	&(N,M)   = M-th capture of the key N levels up (0 = whole key, 1+ = star sub-captures)
func ResolveAmpersand(rhs string, pathEntries []PathEntry) string {
	if !strings.Contains(rhs, "&") {
		return rhs
	}
	var out strings.Builder
	i := 0
	for i < len(rhs) {
		if rhs[i] != '&' {
			out.WriteByte(rhs[i])
			i++
			continue
		}
		_, advance, level, sub := parseAmpToken(rhs[i:])
		i += advance
		out.WriteString(resolveAmp(pathEntries, level, sub))
	}
	return out.String()
}

// parseAmpToken parses one & token from s and returns (token, bytesConsumed, level, subIndex).
func parseAmpToken(s string) (tok string, advance, level, sub int) {
	if len(s) == 0 || s[0] != '&' {
		return "", 0, 0, 0
	}
	i := 1
	if i < len(s) && s[i] == '(' {
		j := strings.Index(s[i:], ")")
		if j < 0 {
			return s, len(s), 0, 0
		}
		inner := s[i+1 : i+j]
		i = i + j + 1
		parts := strings.SplitN(inner, ",", 2)
		level = atoi(strings.TrimSpace(parts[0]))
		if len(parts) == 2 {
			sub = atoi(strings.TrimSpace(parts[1]))
		}
		return s[:i], i, level, sub
	}
	// &N bare integer
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return "&", 1, 0, 0
	}
	level = atoi(s[start:i])
	return s[:i], i, level, 0
}

func resolveAmp(entries []PathEntry, level, sub int) string {
	if level >= len(entries) {
		return ""
	}
	e := entries[level]
	if sub == 0 {
		return e.Key
	}
	idx := sub - 1
	if idx < len(e.Stars) {
		return e.Stars[idx]
	}
	return ""
}

// ResolveHashIndex resolves [#N] tokens in an RHS path, replacing them with
// the match count at the appropriate level.  matchCounts[0] = current level count.
func ResolveHashIndex(rhs string, matchCounts []int) string {
	if !strings.Contains(rhs, "[#") {
		return rhs
	}
	var out strings.Builder
	i := 0
	for i < len(rhs) {
		if i+1 < len(rhs) && rhs[i] == '[' && rhs[i+1] == '#' {
			j := strings.Index(rhs[i:], "]")
			if j < 0 {
				out.WriteByte(rhs[i])
				i++
				continue
			}
			inner := rhs[i+2 : i+j] // digits after #
			level := atoi(inner)
			idx := 0
			if level < len(matchCounts) {
				idx = matchCounts[level]
			}
			out.WriteString(itoa(idx))
			i = i + j + 1
		} else {
			out.WriteByte(rhs[i])
			i++
		}
	}
	return out.String()
}

func atoi(s string) int {
	n := 0
	neg := false
	for i, c := range s {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if neg {
		return -n
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
