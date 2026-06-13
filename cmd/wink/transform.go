package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/clowndaddy/wink"
)

func runTransform(args []string) error {
	fs := flag.NewFlagSet("transform", flag.ContinueOnError)
	noPretty := fs.Bool("u", false, "raw output (no pretty-print)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: wink transform [-u] <spec.json> [input.json]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) < 1 {
		fs.Usage()
		return fmt.Errorf("spec.json is required")
	}

	// Read spec file.
	specJSON, err := os.ReadFile(remaining[0])
	if err != nil {
		return fmt.Errorf("reading spec: %w", err)
	}

	// Read input — file or stdin.
	var inputJSON []byte
	if len(remaining) >= 2 {
		inputJSON, err = os.ReadFile(remaining[1])
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
	} else {
		inputJSON, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	}

	// Parse and run.
	ops, err := wink.ParseChainr(specJSON)
	if err != nil {
		return err
	}

	var input map[string]any
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Errorf("parsing input JSON: %w", err)
	}

	output, err := wink.Transform(input, ops)
	if err != nil {
		return err
	}

	return writeJSON(os.Stdout, output, !*noPretty)
}

// writeJSON marshals v to w, optionally pretty-printed.
func writeJSON(w io.Writer, v any, pretty bool) error {
	var out []byte
	var err error
	if pretty {
		out, err = json.MarshalIndent(v, "", "  ")
	} else {
		out, err = json.Marshal(v)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(out))
	return err
}

// compactJSON returns the compact JSON representation of the given bytes.
func compactJSON(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
