// Command wink is a CLI for the wink JSON transformation library,
// compatible with the Java Jolt CLI interface.
//
// Usage:
//
//	wink transform [-u] <spec.json> [input.json]
//	wink sort      [-u] [input.json]
//	wink diffy     [-s] [-a] <file1.json> [file2.json]
//
// If an input file is omitted, stdin is used.
// Exit code 0 = success; 1 = error or differences found (diffy).
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "transform":
		err = runTransform(os.Args[2:])
	case "sort":
		err = runSort(os.Args[2:])
	case "diffy":
		err = runDiffy(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "wink: unknown sub-command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "wink %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `wink — JSON transformation CLI (Jolt-compatible)

Sub-commands:
  transform [-u] <spec.json> [input.json]
      Apply a Jolt chainr spec to input JSON.
      Reads from stdin if input.json is omitted.
      -u  Raw output (no pretty-print).

  sort [-u] [input.json]
      Sort all map keys alphabetically (~ prefix keys sort first).
      Reads from stdin if input.json is omitted.
      -u  Raw output (no pretty-print).

  diffy [-s] [-a] <file1.json> [file2.json]
      Diff two JSON documents.
      Reads file2 from stdin if omitted.
      -s  Silent: suppress output, use exit code only.
      -a  Ignore array element order when comparing.

Exit codes:
  0  Success (or no differences for diffy)
  1  Error or differences found`)
}
