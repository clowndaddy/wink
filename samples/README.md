# Wink Sample Files

Build the CLI first from the project root:

```bash
make build
```

All commands below assume you are in the project root and the binary is at `bin/wink`.

---

## transform

### Example 1 — E-commerce order restructure

Restructures a raw order into a fulfillment API schema, normalizes the email
to lowercase, applies status defaults, and sorts the output.

```bash
bin/wink transform samples/transform/order-spec.json samples/transform/order-input.json
```

Expected output:
```json
{
  "fulfillment": {
    "destination": {
      "city": "Austin",
      "line1": "123 Main St",
      "postalCode": "78701",
      "region": "TX"
    },
    "lineItems": [
      {
        "description": "Running Shoes",
        "itemCode": "SHOE-001",
        "price": 89.99,
        "quantity": 1
      },
      {
        "description": "Athletic Socks",
        "itemCode": "SOCK-042",
        "price": 12.5,
        "quantity": 3
      }
    ],
    "placedOn": "2024-03-15",
    "priority": "STANDARD",
    "recipient": {
      "email": "alice.johnson@example.com",
      "first": "Alice",
      "last": "Johnson",
      "tier": "gold"
    },
    "referenceId": "ORD-12345",
    "status": "PENDING"
  }
}
```

Raw output (no pretty-print):
```bash
bin/wink transform -u samples/transform/order-spec.json samples/transform/order-input.json
```

From stdin:
```bash
cat samples/transform/order-input.json | bin/wink transform samples/transform/order-spec.json
```

---

### Example 2 — Legacy product feed normalization

Fixes inconsistent cardinality (`tag` is sometimes a string, sometimes a list;
`ratings.score` is sometimes a number, sometimes a list), type-coerces string
fields, computes a derived `avgScore`, and strips internal fields.

```bash
bin/wink transform samples/transform/products-spec.json samples/transform/products-input.json
```

Expected output:
```json
{
  "products": [
    {
      "id": "P001",
      "inStock": true,
      "name": "Widget Pro",
      "price": 49.99,
      "ratings": {
        "avgScore": 4.2,
        "reviewCount": 5,
        "score": [4, 5, 3, 5, 4]
      },
      "tag": ["electronics"]
    },
    {
      "id": "P002",
      "inStock": false,
      "name": "Gadget Lite",
      "price": 19.99,
      "ratings": {
        "avgScore": 3,
        "reviewCount": 1,
        "score": [3]
      },
      "tag": ["electronics", "portable"]
    }
  ]
}
```

---

## sort

Sort all map keys alphabetically. Keys prefixed with `~` are bumped to the top
(Jolt's documented special case — useful for schema/meta keys).

```bash
bin/wink sort samples/sort/unsorted.json
```

Expected output:
```json
{
  "~meta": {
    "author": "system",
    "version": "2.1"
  },
  "apple": "first alphabetically",
  "config": {
    "debug": false,
    "endpoint": "https://api.example.com",
    "retries": 3,
    "timeout": 30
  },
  "mango": "middle",
  "users": [
    {
      "id": 1,
      "name": "Alice",
      "role": "admin"
    },
    {
      "id": 2,
      "name": "Bob",
      "role": "viewer"
    }
  ],
  "zebra": "last alphabetically"
}
```

Raw output:
```bash
bin/wink sort -u samples/sort/unsorted.json
```

From stdin:
```bash
cat samples/sort/unsorted.json | bin/wink sort
```

---

## diffy

### Differences found

Compares two API responses. The second version has a changed role, changed
theme preference, an added permission, and a different lastLogin timestamp.

```bash
bin/wink diffy samples/diffy/response-v1.json samples/diffy/response-v2.json
```

Expected output (exit code 1):
```
Differences found.

Input #1 contained this:
{ ... v1 content ... }

Input #2 contained this:
{ ... v2 content ... }

Specific differences:
  $.user.role: admin != superadmin
  $.user.preferences.theme: dark != light
  $.permissions: array length differs (3 vs 4)
  $.lastLogin: 2024-03-14T09:30:00Z != 2024-03-15T14:22:00Z
```

Silent mode (exit code only, no output):
```bash
bin/wink diffy -s samples/diffy/response-v1.json samples/diffy/response-v2.json
echo "Exit code: $?"
# Exit code: 1
```

### No differences found

```bash
bin/wink diffy samples/diffy/same-a.json samples/diffy/same-b.json
```

Expected output (exit code 0):
```
No differences found.
```

### Ignore array order

```bash
bin/wink diffy -a samples/diffy/same-a.json samples/diffy/same-b.json
```

From stdin:
```bash
cat samples/diffy/response-v2.json | bin/wink diffy samples/diffy/response-v1.json
```
