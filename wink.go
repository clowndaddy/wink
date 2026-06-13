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

// Package wink is a Go implementation of the Java Jolt JSON-to-JSON
// transformation library.  It is spec-file-compatible: existing Jolt chainr
// JSON files work unchanged.
//
// Supported operations: shift, default/defaultr, remove/removr, sort,
// cardinality, modify-overwrite, modify-default, modify-define (and their
// -beta aliases).
package wink

import (
	"encoding/json"
	"fmt"

	"wink/cardinality"
	"wink/defaultop"
	"wink/modify"
	"wink/remove"
	"wink/shift"
	"wink/sorter"
)

// Operation represents one entry in a Jolt chainr spec array.
type Operation struct {
	Operation   string         `json:"operation"`
	Spec        map[string]any `json:"spec,omitempty"`
	Comment     any            `json:"comment,omitempty"`
	Description any            `json:"description,omitempty"`
	Input       any            `json:"input,omitempty"`
	Output      any            `json:"output,omitempty"`
}

// ParseChainr parses a Jolt chainr spec (a JSON array of operation objects).
func ParseChainr(specJSON []byte) ([]Operation, error) {
	var ops []Operation
	if err := json.Unmarshal(specJSON, &ops); err != nil {
		return nil, fmt.Errorf("wink: parse chainr spec: %w", err)
	}
	return ops, nil
}

// Transform applies the chain of operations to input and returns the result.
func Transform(input map[string]any, ops []Operation) (map[string]any, error) {
	result, err := transformInternal(input, ops)
	if err != nil {
		return nil, err
	}
	switch r := result.(type) {
	case map[string]any:
		return r, nil
	case sorter.SortedMap:
		return map[string]any(r), nil
	default:
		return nil, fmt.Errorf("wink: unexpected result type %T", result)
	}
}

// TransformJSON accepts and returns raw JSON bytes.
// chainrSpecJSON is the full chainr spec array.
func TransformJSON(inputJSON, chainrSpecJSON []byte) ([]byte, error) {
	var input map[string]any
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return nil, fmt.Errorf("wink: unmarshal input: %w", err)
	}
	ops, err := ParseChainr(chainrSpecJSON)
	if err != nil {
		return nil, err
	}
	output, err := transformInternal(input, ops)
	if err != nil {
		return nil, err
	}
	return json.Marshal(output)
}

// transformInternal runs the pipeline preserving the concrete type of each
// operation's result so JSON marshalling works correctly (e.g. SortedMap).
func transformInternal(input map[string]any, ops []Operation) (any, error) {
	var current any = input
	for i, op := range ops {
		var currentMap map[string]any
		switch c := current.(type) {
		case map[string]any:
			currentMap = c
		case sorter.SortedMap:
			currentMap = map[string]any(c)
		default:
			return nil, fmt.Errorf("wink: operation[%d]: unexpected intermediate type %T", i, current)
		}

		var err error
		switch op.Operation {
		case "shift":
			current, err = shift.Apply(currentMap, op.Spec)
		case "default", "defaultr":
			current, err = defaultop.Apply(currentMap, op.Spec)
		case "remove", "removr":
			current, err = remove.Apply(currentMap, op.Spec)
		case "sort":
			current, err = sorter.Apply(currentMap)
		case "cardinality":
			current, err = cardinality.Apply(currentMap, op.Spec)
		case "modify-overwrite", "modify-overwrite-beta":
			current, err = modify.Apply(currentMap, op.Spec, modify.Overwrite)
		case "modify-default", "modify-default-beta":
			current, err = modify.Apply(currentMap, op.Spec, modify.Default)
		case "modify-define", "modify-define-beta":
			current, err = modify.Apply(currentMap, op.Spec, modify.Define)
		default:
			return nil, fmt.Errorf("wink: operation[%d]: unknown operation %q", i, op.Operation)
		}
		if err != nil {
			return nil, fmt.Errorf("wink: operation[%d] (%s): %w", i, op.Operation, err)
		}
	}
	return current, nil
}
