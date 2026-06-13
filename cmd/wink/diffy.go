package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
)

func runDiffy(args []string) error {
	fs := flag.NewFlagSet("diffy", flag.ContinueOnError)
	silent := fs.Bool("s", false, "suppress output, use exit code only")
	ignoreArrayOrder := fs.Bool("a", false, "ignore array element order")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: wink diffy [-s] [-a] <file1.json> [file2.json]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) < 1 {
		fs.Usage()
		return fmt.Errorf("file1.json is required")
	}

	// Read input 1.
	data1, err := os.ReadFile(remaining[0])
	if err != nil {
		return fmt.Errorf("reading %s: %w", remaining[0], err)
	}

	// Read input 2 — file or stdin.
	var data2 []byte
	if len(remaining) >= 2 {
		data2, err = os.ReadFile(remaining[1])
		if err != nil {
			return fmt.Errorf("reading %s: %w", remaining[1], err)
		}
	} else {
		data2, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	}

	var v1, v2 any
	if err := json.Unmarshal(data1, &v1); err != nil {
		return fmt.Errorf("parsing %s: %w", remaining[0], err)
	}
	if err := json.Unmarshal(data2, &v2); err != nil {
		return fmt.Errorf("parsing input 2: %w", err)
	}

	diffs := diffValues(v1, v2, "$", *ignoreArrayOrder)

	if len(diffs) == 0 {
		if !*silent {
			fmt.Println("No differences found.")
		}
		return nil
	}

	if !*silent {
		fmt.Println("Differences found.")
		fmt.Println()

		// Show the differing subtrees from each side.
		pretty1, _ := json.MarshalIndent(v1, "", "  ")
		pretty2, _ := json.MarshalIndent(v2, "", "  ")
		fmt.Println("Input #1 contained this:")
		fmt.Println(string(pretty1))
		fmt.Println()
		fmt.Println("Input #2 contained this:")
		fmt.Println(string(pretty2))
		fmt.Println()
		fmt.Println("Specific differences:")
		for _, d := range diffs {
			fmt.Println(" ", d)
		}
	}

	// Exit code 1 signals differences found — return a sentinel.
	os.Exit(1)
	return nil
}

// diffValues recursively compares two JSON values and returns a list of
// human-readable difference descriptions.
func diffValues(v1, v2 any, path string, ignoreArrayOrder bool) []string {
	var diffs []string

	switch a := v1.(type) {
	case map[string]any:
		b, ok := v2.(map[string]any)
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s: type mismatch (object vs %T)", path, v2))
			return diffs
		}
		// Keys in a but not b.
		for k := range a {
			childPath := path + "." + k
			if _, exists := b[k]; !exists {
				diffs = append(diffs, fmt.Sprintf("%s: key present in #1 but missing in #2", childPath))
			} else {
				diffs = append(diffs, diffValues(a[k], b[k], childPath, ignoreArrayOrder)...)
			}
		}
		// Keys in b but not a.
		for k := range b {
			if _, exists := a[k]; !exists {
				diffs = append(diffs, fmt.Sprintf("%s.%s: key missing in #1 but present in #2", path, k))
			}
		}

	case []any:
		b, ok := v2.([]any)
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s: type mismatch (array vs %T)", path, v2))
			return diffs
		}
		if ignoreArrayOrder {
			if !arraysEqualIgnoreOrder(a, b) {
				diffs = append(diffs, fmt.Sprintf("%s: arrays differ (ignoring order)", path))
			}
		} else {
			if len(a) != len(b) {
				diffs = append(diffs, fmt.Sprintf("%s: array length differs (%d vs %d)", path, len(a), len(b)))
			}
			n := len(a)
			if len(b) < n {
				n = len(b)
			}
			for i := 0; i < n; i++ {
				diffs = append(diffs, diffValues(a[i], b[i], fmt.Sprintf("%s[%d]", path, i), ignoreArrayOrder)...)
			}
		}

	default:
		if !reflect.DeepEqual(v1, v2) {
			diffs = append(diffs, fmt.Sprintf("%s: %v != %v", path, v1, v2))
		}
	}

	return diffs
}

// arraysEqualIgnoreOrder checks whether two arrays contain the same elements
// regardless of order, using JSON serialisation as the comparison key.
func arraysEqualIgnoreOrder(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	count := map[string]int{}
	for _, v := range a {
		k, _ := json.Marshal(v)
		count[string(k)]++
	}
	for _, v := range b {
		k, _ := json.Marshal(v)
		count[string(k)]--
		if count[string(k)] < 0 {
			return false
		}
	}
	return true
}

// sortedKeys returns the keys of a map in sorted order (used internally).
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
