# wink

A Go implementation of the Java [Jolt](https://github.com/bazaarvoice/jolt) JSON-to-JSON transformation library. Existing Jolt chainr spec files work unchanged — no migration required.

## Installation

### Library

```bash
go get github.com/clowndaddy/wink
```

### CLI

```bash
go install github.com/clowndaddy/wink/cmd/wink@latest
```

Or build locally:

```bash
make build   # produces bin/wink
```

## CLI Usage

```
wink transform [-u] <spec.json> [input.json]
wink sort      [-u] [input.json]
wink diffy     [-s] [-a] <file1.json> [file2.json]
```

If an input file is omitted, stdin is used. Exit code `0` = success, `1` = error or differences found (diffy).

### transform

Apply a Jolt chainr spec to a JSON document:

```bash
wink transform spec.json input.json

# Raw output (no pretty-print)
wink transform -u spec.json input.json

# From stdin
cat input.json | wink transform spec.json
```

### sort

Sort all map keys alphabetically. Keys prefixed with `~` sort to the top:

```bash
wink sort input.json

# From stdin
cat input.json | wink sort
```

### diffy

Diff two JSON documents:

```bash
wink diffy file1.json file2.json

# Ignore array element order
wink diffy -a file1.json file2.json

# Silent mode — exit code only, no output
wink diffy -s file1.json file2.json
```

## Library Usage

```go
import "github.com/clowndaddy/wink"

// Parse a chainr spec file
ops, err := wink.ParseChainr(specJSON)

// Apply to input
output, err := wink.Transform(input, ops)

// Convenience wrapper for raw JSON bytes
outputJSON, err := wink.TransformJSON(inputJSON, specJSON)
```

## Supported Operations

| Operation | Aliases | Description |
|-----------|---------|-------------|
| `shift` | — | Copy and restructure data from input to output. Any key not shifted disappears. |
| `default` | `defaultr` | Fill in missing keys without overwriting existing values. |
| `remove` | `removr` | Remove keys from the tree. |
| `sort` | — | Sort all map keys alphabetically (`~` prefix keys sort first). |
| `cardinality` | — | Normalize fields to `ONE` (scalar) or `MANY` (array). |
| `modify-overwrite` | `modify-overwrite-beta` | Apply functions to values; always writes. |
| `modify-default` | `modify-default-beta` | Apply functions; writes only if key is absent or null. |
| `modify-define` | `modify-define-beta` | Apply functions; writes only if key is absent. |

## Chainr Spec Format

A spec file is a JSON array of operation objects — identical to the Jolt format:

```json
[
  {
    "operation": "shift",
    "comment": "comments are ignored",
    "spec": {
      "originalKey": "newKey",
      "nested": {
        "field": "top.level.field"
      }
    }
  },
  {
    "operation": "default",
    "spec": {
      "status": "PENDING"
    }
  }
]
```

## Shift Wildcards

| Symbol | Side | Description |
|--------|------|-------------|
| `*` | LHS | Match all keys (non-greedy, lowest priority) |
| `prefix-*` | LHS | Partial glob; captures the starred portion |
| `a\|b` | LHS | OR: match either literal |
| `&`, `&N`, `&(N,M)` | LHS + RHS | Dereference path key at level N, capture M |
| `$` | LHS | Use matched input key as the output value |
| `#literal` | LHS | Write a literal string as the output value |
| `@` | LHS | Copy the current input value to the output |
| `@(N,key)` | LHS | Look up key N levels up in the input tree |
| `\X` | LHS | Escape: treat X as a literal character |
| `key[]` | RHS | Append to an output array |
| `key[N]` | RHS | Write to a specific array index |
| `[#N]` | RHS | Use match count at level N as the array index |

## Modify Functions

### String
`toLower` `toUpper` `trim` `concat` `join` `split` `substring` `leftPad` `rightPad` `replace` `replaceAll`

### Math
`min` `max` `abs` `avg` `intSum` `doubleSum` `longSum` `intSubtract` `doubleSubtract` `longSubtract` `divide` `divideAndRound` `multiply` `multiplyAndRound`

### Type Conversion
`toInteger` `toDouble` `toLong` `toBoolean` `toString` `size`

### List
`firstElement` `lastElement` `elementAt` `toList` `sort`

### Object
`squashNulls` `recursivelySquashNulls` `squashDuplicates`

### Date
`now` `nowEpochMillis` `fromEpochMilli` `toEpochMilli` `dateAdd` `dateSubstract` `formatDate`

### Utility
`noop` `isPresent` `notNull` `isNull` `uuid` `defaultValue`

## Sample Files

The `samples/` directory contains ready-to-run examples for all three CLI commands. See [`samples/README.md`](samples/README.md) for the full walkthrough with expected output.

```bash
# E-commerce order restructure
bin/wink transform samples/transform/order-spec.json samples/transform/order-input.json

# Legacy product feed normalization
bin/wink transform samples/transform/products-spec.json samples/transform/products-input.json

# Sort
bin/wink sort samples/sort/unsorted.json

# Diff
bin/wink diffy samples/diffy/response-v1.json samples/diffy/response-v2.json
```

## Project Structure

```
wink/
├── wink.go                    # Public API: Transform, TransformJSON, ParseChainr
├── cmd/wink/                  # CLI (transform, sort, diffy sub-commands)
├── shift/                     # Shift operation
├── defaultop/                 # Default operation
├── remove/                    # Remove operation
├── sorter/                    # Sort operation
├── cardinality/               # Cardinality operation
├── modify/                    # Modify-overwrite/default/define + function library
├── internal/
│   ├── pathutil/              # Dot-notation + bracket path writer
│   └── wildcards/             # LHS matching and & reference resolution
└── samples/                   # Example spec and input files
```

## License

Apache 2.0 — the same license as the original Jolt library.
