package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"wink/sorter"
)

func runSort(args []string) error {
	fs := flag.NewFlagSet("sort", flag.ContinueOnError)
	noPretty := fs.Bool("u", false, "raw output (no pretty-print)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: wink sort [-u] [input.json]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Read input — file or stdin.
	var inputJSON []byte
	var err error
	if len(fs.Args()) >= 1 {
		inputJSON, err = os.ReadFile(fs.Args()[0])
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
	} else {
		inputJSON, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	}

	var input map[string]any
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Errorf("parsing input JSON: %w", err)
	}

	sorted, err := sorter.Apply(input)
	if err != nil {
		return err
	}

	return writeJSON(os.Stdout, sorted, !*noPretty)
}
