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

// Package modify implements the Jolt modify-overwrite, modify-default, and
// modify-define transforms, along with their full built-in function library.
package modify

import (
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WriteMode controls when a modify operation writes its computed value.
type WriteMode int

const (
	// Overwrite always writes (modify-overwrite).
	Overwrite WriteMode = iota
	// Default writes only if the key is absent or null (modify-default).
	Default
	// Define writes only if the key is absent entirely (modify-define).
	Define
)

// evalFunction evaluates a Jolt function expression like "=concat(@(1,a),' ',@(1,b))".
// context is the current map being processed (for @ lookups).
func evalFunction(expr string, context map[string]any) (any, error) {
	if !strings.HasPrefix(expr, "=") {
		// Literal value.
		return expr, nil
	}
	expr = expr[1:] // strip "="

	// Parse function name and raw args string.
	parenIdx := strings.Index(expr, "(")
	if parenIdx < 0 {
		return nil, fmt.Errorf("modify: invalid function expression: %q", expr)
	}
	fnName := strings.TrimSpace(expr[:parenIdx])
	argsStr := expr[parenIdx+1:]
	// Strip trailing ")"
	if last := strings.LastIndex(argsStr, ")"); last >= 0 {
		argsStr = argsStr[:last]
	}

	// Resolve args (may contain @(N,key) lookups or nested literals).
	args, err := parseArgs(argsStr, context)
	if err != nil {
		return nil, fmt.Errorf("modify: function %q args: %w", fnName, err)
	}

	return callFunction(fnName, args, context)
}

// parseArgs splits comma-separated args, resolving @(N,key) references.
func parseArgs(argsStr string, context map[string]any) ([]any, error) {
	if strings.TrimSpace(argsStr) == "" {
		return nil, nil
	}

	// Split on commas that are not inside parentheses or quotes.
	parts := splitArgs(argsStr)
	args := make([]any, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		resolved, err := resolveArg(p, context)
		if err != nil {
			return nil, err
		}
		args = append(args, resolved)
	}
	return args, nil
}

// splitArgs splits argsStr on top-level commas.
func splitArgs(s string) []string {
	var parts []string
	depth := 0
	inSingle := false
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\'' && !inSingle {
			inSingle = true
		} else if ch == '\'' && inSingle {
			inSingle = false
		} else if !inSingle {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
			} else if ch == ',' && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// resolveArg resolves a single argument token.
func resolveArg(arg string, context map[string]any) (any, error) {
	arg = strings.TrimSpace(arg)

	// Single-quoted string literal: 'value'
	if strings.HasPrefix(arg, "'") && strings.HasSuffix(arg, "'") {
		return arg[1 : len(arg)-1], nil
	}

	// @(N,key) lookup
	if strings.HasPrefix(arg, "@(") && strings.HasSuffix(arg, ")") {
		inner := arg[2 : len(arg)-1]
		parts := strings.SplitN(inner, ",", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[1])
			if v, ok := context[key]; ok {
				return v, nil
			}
			return nil, nil
		}
	}

	// @ alone means the current value — not applicable here, return nil.
	if arg == "@" {
		return nil, nil
	}

	// Bare number.
	if n, err := strconv.ParseFloat(arg, 64); err == nil {
		return n, nil
	}

	// Bare boolean.
	if arg == "true" {
		return true, nil
	}
	if arg == "false" {
		return false, nil
	}

	// Bare null.
	if arg == "null" {
		return nil, nil
	}

	// Nested function call.
	if strings.Contains(arg, "(") {
		return evalFunction("="+arg, context)
	}

	// Bare key reference (look up in context).
	if v, ok := context[arg]; ok {
		return v, nil
	}

	// Return as string.
	return arg, nil
}

// callFunction dispatches to the appropriate built-in function.
func callFunction(name string, args []any, context map[string]any) (any, error) {
	switch name {
	// ── String functions ──────────────────────────────────────────────────
	case "toLower":
		return applyStr(args, strings.ToLower)
	case "toUpper":
		return applyStr(args, strings.ToUpper)
	case "trim":
		return applyStr(args, strings.TrimSpace)
	case "concat":
		var sb strings.Builder
		for _, a := range args {
			sb.WriteString(toString(a))
		}
		return sb.String(), nil
	case "join":
		if len(args) < 1 {
			return "", nil
		}
		sep := toString(args[0])
		parts := make([]string, 0, len(args)-1)
		for _, a := range args[1:] {
			parts = append(parts, toString(a))
		}
		return strings.Join(parts, sep), nil
	case "split":
		if len(args) < 2 {
			return nil, fmt.Errorf("split requires 2 args")
		}
		sep := toString(args[0])
		str := toString(args[1])
		parts := strings.Split(str, sep)
		out := make([]any, len(parts))
		for i, p := range parts {
			out[i] = p
		}
		return out, nil
	case "substring":
		if len(args) < 3 {
			return nil, fmt.Errorf("substring requires 3 args")
		}
		str := toString(args[0])
		start := int(toFloat(args[1]))
		end := int(toFloat(args[2]))
		if start < 0 {
			start = 0
		}
		if end > len(str) {
			end = len(str)
		}
		return str[start:end], nil
	case "leftPad":
		if len(args) < 3 {
			return nil, fmt.Errorf("leftPad requires 3 args")
		}
		str := toString(args[0])
		width := int(toFloat(args[1]))
		pad := toString(args[2])
		for len(str) < width {
			str = pad + str
		}
		return str, nil
	case "rightPad":
		if len(args) < 3 {
			return nil, fmt.Errorf("rightPad requires 3 args")
		}
		str := toString(args[0])
		width := int(toFloat(args[1]))
		pad := toString(args[2])
		for len(str) < width {
			str = str + pad
		}
		return str, nil
	case "replace":
		if len(args) < 3 {
			return nil, fmt.Errorf("replace requires 3 args")
		}
		return strings.Replace(toString(args[0]), toString(args[1]), toString(args[2]), 1), nil
	case "replaceAll":
		if len(args) < 3 {
			return nil, fmt.Errorf("replaceAll requires 3 args")
		}
		re, err := regexp.Compile(toString(args[1]))
		if err != nil {
			return nil, err
		}
		return re.ReplaceAllString(toString(args[0]), toString(args[2])), nil

	// ── Math functions ────────────────────────────────────────────────────
	case "min":
		args = flattenListArg(args)
		if len(args) == 0 {
			return nil, nil
		}
		m := toFloat(args[0])
		for _, a := range args[1:] {
			if v := toFloat(a); v < m {
				m = v
			}
		}
		return m, nil
	case "max":
		args = flattenListArg(args)
		if len(args) == 0 {
			return nil, nil
		}
		m := toFloat(args[0])
		for _, a := range args[1:] {
			if v := toFloat(a); v > m {
				m = v
			}
		}
		return m, nil
	case "abs":
		if len(args) < 1 {
			return nil, nil
		}
		return math.Abs(toFloat(args[0])), nil
	case "avg":
		if len(args) == 0 {
			return nil, nil
		}
		// If a single list argument is passed, average its elements.
		if len(args) == 1 {
			if arr, ok := args[0].([]any); ok {
				if len(arr) == 0 {
					return nil, nil
				}
				sum := 0.0
				for _, a := range arr {
					sum += toFloat(a)
				}
				return sum / float64(len(arr)), nil
			}
		}
		sum := 0.0
		for _, a := range args {
			sum += toFloat(a)
		}
		return sum / float64(len(args)), nil
	case "intSum":
		args = flattenListArg(args)
		sum := 0.0
		for _, a := range args {
			sum += toFloat(a)
		}
		return int64(sum), nil
	case "doubleSum":
		args = flattenListArg(args)
		sum := 0.0
		for _, a := range args {
			sum += toFloat(a)
		}
		return sum, nil
	case "longSum":
		args = flattenListArg(args)
		sum := int64(0)
		for _, a := range args {
			sum += int64(toFloat(a))
		}
		return sum, nil
	case "intSubtract":
		if len(args) < 2 {
			return nil, fmt.Errorf("intSubtract requires 2 args")
		}
		return int64(toFloat(args[0])) - int64(toFloat(args[1])), nil
	case "doubleSubtract":
		if len(args) < 2 {
			return nil, fmt.Errorf("doubleSubtract requires 2 args")
		}
		return toFloat(args[0]) - toFloat(args[1]), nil
	case "longSubtract":
		if len(args) < 2 {
			return nil, fmt.Errorf("longSubtract requires 2 args")
		}
		return int64(toFloat(args[0])) - int64(toFloat(args[1])), nil
	case "divide":
		if len(args) < 2 {
			return nil, fmt.Errorf("divide requires 2 args")
		}
		return toFloat(args[0]) / toFloat(args[1]), nil
	case "divideAndRound":
		if len(args) < 3 {
			return nil, fmt.Errorf("divideAndRound requires 3 args")
		}
		result := toFloat(args[0]) / toFloat(args[1])
		decimals := int(toFloat(args[2]))
		factor := math.Pow(10, float64(decimals))
		return math.Round(result*factor) / factor, nil
	case "multiply":
		if len(args) < 2 {
			return nil, fmt.Errorf("multiply requires 2 args")
		}
		return toFloat(args[0]) * toFloat(args[1]), nil
	case "multiplyAndRound":
		if len(args) < 3 {
			return nil, fmt.Errorf("multiplyAndRound requires 3 args")
		}
		result := toFloat(args[0]) * toFloat(args[1])
		decimals := int(toFloat(args[2]))
		factor := math.Pow(10, float64(decimals))
		return math.Round(result*factor) / factor, nil

	// ── Type conversion ───────────────────────────────────────────────────
	case "toInteger":
		if len(args) < 1 {
			return nil, nil
		}
		return int64(toFloat(args[0])), nil
	case "toDouble":
		if len(args) < 1 {
			return nil, nil
		}
		return toFloat(args[0]), nil
	case "toLong":
		if len(args) < 1 {
			return nil, nil
		}
		return int64(toFloat(args[0])), nil
	case "toBoolean":
		if len(args) < 1 {
			return nil, nil
		}
		s := strings.ToLower(toString(args[0]))
		return s == "true" || s == "1" || s == "yes", nil
	case "toString":
		if len(args) < 1 {
			return nil, nil
		}
		return toString(args[0]), nil
	case "size":
		if len(args) < 1 {
			return nil, nil
		}
		switch v := args[0].(type) {
		case string:
			return int64(len(v)), nil
		case []any:
			return int64(len(v)), nil
		case map[string]any:
			return int64(len(v)), nil
		}
		return int64(0), nil

	// ── List functions ────────────────────────────────────────────────────
	case "firstElement":
		if len(args) < 1 {
			return nil, nil
		}
		if arr, ok := args[0].([]any); ok && len(arr) > 0 {
			return arr[0], nil
		}
		return args[0], nil
	case "lastElement":
		if len(args) < 1 {
			return nil, nil
		}
		if arr, ok := args[0].([]any); ok && len(arr) > 0 {
			return arr[len(arr)-1], nil
		}
		return args[0], nil
	case "elementAt":
		if len(args) < 2 {
			return nil, nil
		}
		if arr, ok := args[0].([]any); ok {
			idx := int(toFloat(args[1]))
			if idx >= 0 && idx < len(arr) {
				return arr[idx], nil
			}
		}
		return nil, nil
	case "toList":
		if len(args) < 1 {
			return []any{}, nil
		}
		if arr, ok := args[0].([]any); ok {
			return arr, nil
		}
		return []any{args[0]}, nil
	case "sort":
		if len(args) < 1 {
			return []any{}, nil
		}
		arr, ok := args[0].([]any)
		if !ok {
			return args[0], nil
		}
		sorted := make([]any, len(arr))
		copy(sorted, arr)
		sortAny(sorted)
		return sorted, nil

	// ── Object functions ──────────────────────────────────────────────────
	case "squashNulls":
		if len(args) < 1 {
			// Applied to context itself.
			for k, v := range context {
				if v == nil {
					delete(context, k)
				}
			}
			return context, nil
		}
		if m, ok := args[0].(map[string]any); ok {
			for k, v := range m {
				if v == nil {
					delete(m, k)
				}
			}
			return m, nil
		}
		return args[0], nil
	case "recursivelySquashNulls":
		target := context
		if len(args) > 0 {
			if m, ok := args[0].(map[string]any); ok {
				target = m
			}
		}
		recursivelySquashNulls(target)
		return target, nil
	case "squashDuplicates":
		if len(args) < 1 {
			return nil, nil
		}
		if arr, ok := args[0].([]any); ok {
			seen := map[string]bool{}
			var out []any
			for _, v := range arr {
				k := fmt.Sprintf("%v", v)
				if !seen[k] {
					seen[k] = true
					out = append(out, v)
				}
			}
			return out, nil
		}
		return args[0], nil

	// ── Date functions ────────────────────────────────────────────────────
	case "now":
		return time.Now().UTC().Format(time.RFC3339), nil
	case "nowEpochMillis":
		return time.Now().UnixMilli(), nil
	case "fromEpochMilli":
		if len(args) < 1 {
			return nil, nil
		}
		ms := int64(toFloat(args[0]))
		return time.UnixMilli(ms).UTC().Format(time.RFC3339), nil
	case "toEpochMilli":
		if len(args) < 1 {
			return nil, nil
		}
		t, err := time.Parse("2006-01-02", toString(args[0]))
		if err != nil {
			return nil, err
		}
		return t.UnixMilli(), nil
	case "dateAdd":
		if len(args) < 3 {
			return nil, fmt.Errorf("dateAdd requires 3 args")
		}
		return dateArithmetic(toString(args[0]), int(toFloat(args[1])), toString(args[2]), true)
	case "dateSubstract":
		if len(args) < 3 {
			return nil, fmt.Errorf("dateSubstract requires 3 args")
		}
		return dateArithmetic(toString(args[0]), int(toFloat(args[1])), toString(args[2]), false)
	case "formatDate":
		if len(args) < 3 {
			return nil, fmt.Errorf("formatDate requires at least 3 args")
		}
		return formatDate(toString(args[0]), toString(args[1]), toString(args[2]))

	// ── Utility functions ─────────────────────────────────────────────────
	case "noop":
		if len(args) > 0 {
			return args[0], nil
		}
		return nil, nil
	case "isPresent":
		if len(args) < 1 {
			return false, nil
		}
		return args[0] != nil, nil
	case "notNull":
		if len(args) < 1 {
			return false, nil
		}
		return args[0] != nil, nil
	case "isNull":
		if len(args) < 1 {
			return true, nil
		}
		return args[0] == nil, nil
	case "uuid":
		return generateUUID(), nil
	case "defaultValue":
		// =defaultValue('x') returns 'x' — used in modify-define context.
		if len(args) > 0 {
			return args[0], nil
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("modify: unknown function %q", name)
	}
}

// flattenListArg expands a single-[]any-arg into individual elements.
// This allows functions like avg, min, max to accept either a list variable
// or individual scalar arguments: avg([1,2,3]) and avg(1,2,3) both work.
func flattenListArg(args []any) []any {
	if len(args) == 1 {
		if arr, ok := args[0].([]any); ok {
			return arr
		}
	}
	return args
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func applyStr(args []any, fn func(string) string) (any, error) {
	if len(args) < 1 {
		return nil, nil
	}
	return fn(toString(args[0])), nil
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == math.Trunc(val) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case bool:
		if val {
			return 1
		}
		return 0
	}
	return 0
}

func sortAny(arr []any) {
	// Simple string-based sort.
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0 && toString(arr[j]) < toString(arr[j-1]); j-- {
			arr[j], arr[j-1] = arr[j-1], arr[j]
		}
	}
}

func recursivelySquashNulls(m map[string]any) {
	for k, v := range m {
		if v == nil {
			delete(m, k)
		} else if child, ok := v.(map[string]any); ok {
			recursivelySquashNulls(child)
		}
	}
}

// generateUUID generates a random UUID v4.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// javaToGoDateFormat converts Java SimpleDateFormat patterns to Go time layout.
func javaToGoDateFormat(pattern string) string {
	replacements := []struct{ java, goFmt string }{
		{"yyyy", "2006"}, {"yy", "06"},
		{"MM", "01"}, {"M", "1"},
		{"dd", "02"}, {"d", "2"},
		{"HH", "15"}, {"mm", "04"}, {"ss", "05"},
		{"SSS", "000"},
		{"XXX", "-07:00"}, {"XX", "-0700"}, {"X", "-07"},
		{"'T'", "T"}, {"'Z'", "Z"},
	}
	result := pattern
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.java, r.goFmt)
	}
	return result
}

func formatDate(dateStr, fromPattern, toPattern string) (string, error) {
	fromLayout := javaToGoDateFormat(fromPattern)
	toLayout := javaToGoDateFormat(toPattern)
	t, err := time.Parse(fromLayout, dateStr)
	if err != nil {
		return "", fmt.Errorf("formatDate: parse %q with %q: %w", dateStr, fromLayout, err)
	}
	return t.Format(toLayout), nil
}

func dateArithmetic(dateStr string, amount int, unit string, add bool) (string, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		t, err = time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return "", fmt.Errorf("dateAdd: cannot parse %q", dateStr)
		}
	}
	if !add {
		amount = -amount
	}
	switch strings.ToUpper(unit) {
	case "DAYS", "DAY":
		t = t.AddDate(0, 0, amount)
	case "MONTHS", "MONTH":
		t = t.AddDate(0, amount, 0)
	case "YEARS", "YEAR":
		t = t.AddDate(amount, 0, 0)
	case "HOURS", "HOUR":
		t = t.Add(time.Duration(amount) * time.Hour)
	case "MINUTES", "MINUTE":
		t = t.Add(time.Duration(amount) * time.Minute)
	}
	return t.Format("2006-01-02"), nil
}
