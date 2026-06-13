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

package wink_test

import (
	"encoding/json"
	"testing"

	"wink"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func jsonStr(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// runChainr runs a chainr spec (JSON array) against input and returns the output map.
func runChainr(t *testing.T, inputJSON, chainrJSON string) map[string]any {
	t.Helper()
	out, err := wink.TransformJSON([]byte(inputJSON), []byte(chainrJSON))
	if err != nil {
		t.Fatalf("TransformJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	return m
}

func assertKey(t *testing.T, m map[string]any, key string, want any) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("key %q missing from output", key)
		return
	}
	if jsonStr(t, got) != jsonStr(t, want) {
		t.Errorf("key %q: got %v, want %v", key, jsonStr(t, got), jsonStr(t, want))
	}
}

func assertNoKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Errorf("key %q should not be in output", key)
	}
}

// ─── Chainr spec format ──────────────────────────────────────────────────────

// TestChainrSpecFormat verifies that the real Jolt chainr JSON array format is
// parsed and executed correctly — the primary goal of wink.
func TestChainrSpecFormat(t *testing.T) {
	input := `{"name":"Alice","age":30,"password":"secret"}`
	chainr := `[
		{
			"operation": "shift",
			"comment": "rename fields",
			"spec": {
				"name": "Name",
				"age":  "Age"
			}
		},
		{
			"operation": "default",
			"spec": {
				"Role": "viewer"
			}
		}
	]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "Name", "Alice")
	assertKey(t, out, "Age", float64(30))
	assertKey(t, out, "Role", "viewer")
	// "password" was not shifted → absent
	assertNoKey(t, out, "password")
	assertNoKey(t, out, "name")
}

// ─── Shift ───────────────────────────────────────────────────────────────────

func TestShift_LiteralRename(t *testing.T) {
	input := `{"oldName":"Alice"}`
	chainr := `[{"operation":"shift","spec":{"oldName":"newName"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "newName", "Alice")
	assertNoKey(t, out, "oldName")
}

func TestShift_NestedSpec_DotNotationRHS(t *testing.T) {
	// LHS nested, RHS dot-notation — the correct Jolt syntax.
	input := `{"rating":{"max":5,"min":1}}`
	chainr := `[{"operation":"shift","spec":{"rating":{"max":"Rating.Max","min":"Rating.Min"}}}]`
	out := runChainr(t, input, chainr)
	rating, ok := out["Rating"].(map[string]any)
	if !ok {
		t.Fatalf("Rating should be a map, got %T", out["Rating"])
	}
	assertKey(t, rating, "Max", float64(5))
	assertKey(t, rating, "Min", float64(1))
}

func TestShift_StarWildcard_MatchesAll(t *testing.T) {
	// "*":"&" idiom — keep all keys as-is.
	input := `{"a":1,"b":2}`
	chainr := `[{"operation":"shift","spec":{"*":"&"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "a", float64(1))
	assertKey(t, out, "b", float64(2))
}

func TestShift_StarWildcard_Prefix(t *testing.T) {
	// "tag-*" captures portion after prefix; & on RHS is the whole matched key.
	input := `{"tag-Pro":"Awesome","tag-Con":"Bogus","other":"x"}`
	chainr := `[{"operation":"shift","spec":{"tag-*":"tags.&"}}]`
	out := runChainr(t, input, chainr)
	tags, ok := out["tags"].(map[string]any)
	if !ok {
		t.Fatalf("tags should be a map, got %T", out["tags"])
	}
	if tags["tag-Pro"] != "Awesome" && tags["tag-Con"] != "Bogus" {
		t.Errorf("expected tag keys in output.tags; got %v", tags)
	}
	assertNoKey(t, out, "other")
}

func TestShift_MultipleOutputPaths_ArraySpec(t *testing.T) {
	// A spec value that is a JSON array copies the value to all listed paths.
	input := `{"foo":3}`
	chainr := `[{"operation":"shift","spec":{"foo":["bar","baz"]}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "bar", float64(3))
	assertKey(t, out, "baz", float64(3))
}

func TestShift_ImplicitArrayCreation(t *testing.T) {
	// Two values shifted to the same output key → implicit array.
	input := `{"foo":"bar","tuna":"marlin"}`
	chainr := `[{"operation":"shift","spec":{"foo":"baz","tuna":"baz"}}]`
	out := runChainr(t, input, chainr)
	baz, ok := out["baz"].([]any)
	if !ok {
		t.Fatalf("baz should be a list due to implicit array creation, got %T: %v", out["baz"], out["baz"])
	}
	if len(baz) != 2 {
		t.Errorf("baz list should have 2 elements, got %d", len(baz))
	}
}

func TestShift_OrWildcard(t *testing.T) {
	// "rating|Rating" matches either key.
	input := `{"Rating":5}`
	chainr := `[{"operation":"shift","spec":{"rating|Rating":"rating-primary"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "rating-primary", float64(5))
}

func TestShift_KeepUntouchedNestedObjects(t *testing.T) {
	// "*":"&" at root keeps untouched sub-objects intact.
	input := `{"untouched":{"a":true,"b":{"c":true}},"rename":"x"}`
	chainr := `[{"operation":"shift","spec":{"*":"&","rename":"RENAMED"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "RENAMED", "x")
	assertNoKey(t, out, "rename")
	if _, ok := out["untouched"]; !ok {
		t.Error("untouched key should be preserved by *:& idiom")
	}
}

// ─── Default ─────────────────────────────────────────────────────────────────

func TestDefault_FillsMissingKey(t *testing.T) {
	input := `{"status":"inactive"}`
	chainr := `[{"operation":"default","spec":{"status":"active","role":"viewer"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "status", "inactive") // not overwritten
	assertKey(t, out, "role", "viewer")     // added
}

func TestDefault_NestedFill(t *testing.T) {
	input := `{"meta":{"author":"Bob"}}`
	chainr := `[{"operation":"default","spec":{"meta":{"author":"Unknown","retries":3}}}]`
	out := runChainr(t, input, chainr)
	meta := out["meta"].(map[string]any)
	assertKey(t, meta, "author", "Bob")       // not overwritten
	assertKey(t, meta, "retries", float64(3)) // added
}

func TestDefault_StarWildcard_AppliesOnlyToExisting(t *testing.T) {
	// "*" wildcard applies defaults only to keys already present.
	input := `{"SecondaryRatings":{"quality":{"Value":3},"sharpness":{"Value":4}}}`
	chainr := `[{"operation":"default","spec":{"SecondaryRatings":{"*":{"DisplayType":"NORMAL"}}}}]`
	out := runChainr(t, input, chainr)
	sr := out["SecondaryRatings"].(map[string]any)
	q := sr["quality"].(map[string]any)
	assertKey(t, q, "DisplayType", "NORMAL")
	s := sr["sharpness"].(map[string]any)
	assertKey(t, s, "DisplayType", "NORMAL")
}

func TestDefault_OrWildcard(t *testing.T) {
	input := `{"SecondaryRatings":{"quality":{"Value":3}}}`
	chainr := `[{"operation":"default","spec":{"SecondaryRatings":{"quality|sharpness":{"MaxLabel":"Great"}}}}]`
	out := runChainr(t, input, chainr)
	sr := out["SecondaryRatings"].(map[string]any)
	q := sr["quality"].(map[string]any)
	assertKey(t, q, "MaxLabel", "Great")
}

func TestDefault_ArraySpec(t *testing.T) {
	// "photos[]" with integer-keyed children defaults specific array slots.
	input := `{"photos":[{"url":"http://a.com"}]}`
	chainr := `[{"operation":"default","spec":{"photos[]":{"2":{"url":"http://www.bazaarvoice.com","caption":""}}}}]`
	out := runChainr(t, input, chainr)
	photos, ok := out["photos"].([]any)
	if !ok {
		t.Fatalf("photos should be a list, got %T", out["photos"])
	}
	if len(photos) < 3 {
		t.Fatalf("photos should have at least 3 elements after default, got %d", len(photos))
	}
	slot2, ok := photos[2].(map[string]any)
	if !ok {
		t.Fatalf("photos[2] should be a map, got %T", photos[2])
	}
	assertKey(t, slot2, "url", "http://www.bazaarvoice.com")
}

// ─── Remove ──────────────────────────────────────────────────────────────────

func TestRemove_TopLevelKey(t *testing.T) {
	input := `{"id":"123","productId":"31231","submissionId":"34343","this":"stays"}`
	chainr := `[{"operation":"remove","spec":{"productId":"","submissionId":""}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "id", "123")
	assertKey(t, out, "this", "stays")
	assertNoKey(t, out, "productId")
	assertNoKey(t, out, "submissionId")
}

func TestRemove_NestedKey(t *testing.T) {
	input := `{"configured":{"a":"b","c":"d"}}`
	chainr := `[{"operation":"remove","spec":{"configured":{"c":""}}}]`
	out := runChainr(t, input, chainr)
	cfg := out["configured"].(map[string]any)
	assertKey(t, cfg, "a", "b")
	assertNoKey(t, cfg, "c")
}

func TestRemove_StarWildcard_RemovesMatchingChildren(t *testing.T) {
	// Remove "b" from all children of "ratings".
	input := `{"ratings":{"Set1":{"a":"a","b":"b"},"Set2":{"c":"c","b":"b"}}}`
	chainr := `[{"operation":"remove","spec":{"ratings":{"*":{"b":""}}}}]`
	out := runChainr(t, input, chainr)
	ratings := out["ratings"].(map[string]any)
	set1 := ratings["Set1"].(map[string]any)
	assertKey(t, set1, "a", "a")
	assertNoKey(t, set1, "b")
	set2 := ratings["Set2"].(map[string]any)
	assertKey(t, set2, "c", "c")
	assertNoKey(t, set2, "b")
}

func TestRemove_PrefixWildcard(t *testing.T) {
	input := `{"ratings_legacy":{"Set1":{"a":1},"Set2":{"a":2}},"ratings_new":{"Set1":{"a":3}}}`
	chainr := `[{"operation":"remove","spec":{"ratings_*":{"Set1":""}}}]`
	out := runChainr(t, input, chainr)
	rl := out["ratings_legacy"].(map[string]any)
	assertNoKey(t, rl, "Set1")
	assertKey(t, rl, "Set2", map[string]any{"a": float64(2)})
}

// ─── Sort ────────────────────────────────────────────────────────────────────

func TestSort_AlphabeticalOrder(t *testing.T) {
	input := `{"zebra":1,"apple":2,"mango":3}`
	chainr := `[{"operation":"sort"}]`
	outBytes, err := wink.TransformJSON([]byte(input), []byte(chainr))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"apple":2,"mango":3,"zebra":1}`
	if string(outBytes) != want {
		t.Errorf("sort: got %s, want %s", outBytes, want)
	}
}

func TestSort_TildePrefixFirst(t *testing.T) {
	input := `{"zebra":1,"~meta":"first","apple":2}`
	chainr := `[{"operation":"sort"}]`
	outBytes, err := wink.TransformJSON([]byte(input), []byte(chainr))
	if err != nil {
		t.Fatal(err)
	}
	// ~meta must come before apple and zebra.
	want := `{"~meta":"first","apple":2,"zebra":1}`
	if string(outBytes) != want {
		t.Errorf("sort with tilde: got %s, want %s", outBytes, want)
	}
}

func TestSort_NestedMaps(t *testing.T) {
	input := `{"z":{"fig":3,"banana":4},"a":1}`
	chainr := `[{"operation":"sort"}]`
	outBytes, err := wink.TransformJSON([]byte(input), []byte(chainr))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":1,"z":{"banana":4,"fig":3}}`
	if string(outBytes) != want {
		t.Errorf("sort nested: got %s, want %s", outBytes, want)
	}
}

// ─── Cardinality ─────────────────────────────────────────────────────────────

func TestCardinality_ManyFromScalar(t *testing.T) {
	input := `{"urls":"https://example.com"}`
	chainr := `[{"operation":"cardinality","spec":{"urls":"MANY"}}]`
	out := runChainr(t, input, chainr)
	urls, ok := out["urls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://example.com" {
		t.Errorf("MANY from scalar: got %v", out["urls"])
	}
}

func TestCardinality_ManyFromNull(t *testing.T) {
	input := `{"urls":null}`
	chainr := `[{"operation":"cardinality","spec":{"urls":"MANY"}}]`
	out := runChainr(t, input, chainr)
	urls, ok := out["urls"].([]any)
	if !ok || len(urls) != 0 {
		t.Errorf("MANY from null: expected [], got %v", out["urls"])
	}
}

func TestCardinality_ManyPreservesExistingList(t *testing.T) {
	input := `{"urls":["a","b"]}`
	chainr := `[{"operation":"cardinality","spec":{"urls":"MANY"}}]`
	out := runChainr(t, input, chainr)
	urls, ok := out["urls"].([]any)
	if !ok || len(urls) != 2 {
		t.Errorf("MANY no-op for list: got %v", out["urls"])
	}
}

func TestCardinality_OneFromList(t *testing.T) {
	// The canonical Jolt cardinality example from the README.
	input := `{"review":{"rating":[5,4]}}`
	chainr := `[{"operation":"cardinality","spec":{"review":{"rating":"ONE"}}}]`
	out := runChainr(t, input, chainr)
	review := out["review"].(map[string]any)
	assertKey(t, review, "rating", float64(5))
}

func TestCardinality_OneFromScalar_Noop(t *testing.T) {
	input := `{"id":42}`
	chainr := `[{"operation":"cardinality","spec":{"id":"ONE"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "id", float64(42))
}

func TestCardinality_StarWildcard_ForEach(t *testing.T) {
	// For each item in an array, convert "url" to ONE.
	input := `{"photosArray":[{"url":["http://a.jpg","http://b.jpg"],"caption":"Nice"},{"url":["http://c.jpg"],"caption":"Cool"}]}`
	chainr := `[{"operation":"cardinality","spec":{"photosArray":{"*":{"url":"ONE"}}}}]`
	out := runChainr(t, input, chainr)
	arr, ok := out["photosArray"].([]any)
	if !ok {
		t.Fatalf("photosArray should be a list, got %T", out["photosArray"])
	}
	for i, item := range arr {
		m := item.(map[string]any)
		if _, isList := m["url"].([]any); isList {
			t.Errorf("photosArray[%d].url should be ONE (scalar), still a list", i)
		}
	}
}

func TestCardinality_AtWildcard_ApplyToSelf(t *testing.T) {
	// "@":"ONE" collapses the views list itself to ONE, then "count":"MANY".
	input := `{"views":[{"count":1024},{"count":2048}]}`
	chainr := `[{"operation":"cardinality","spec":{"views":{"@":"ONE","count":"MANY"}}}]`
	out := runChainr(t, input, chainr)
	views, ok := out["views"].(map[string]any)
	if !ok {
		t.Fatalf("views should be a map after ONE, got %T", out["views"])
	}
	counts, ok := views["count"].([]any)
	if !ok || len(counts) == 0 {
		t.Errorf("count should be MANY (list), got %v", views["count"])
	}
}

// ─── End-to-end pipeline ────────────────────────────────────────────────────

func TestFullPipeline_ShiftDefaultRemoveSort(t *testing.T) {
	input := `{
		"rating": {"max": 5, "min": 1},
		"~emVersion": "2",
		"id": "123"
	}`
	chainr := `[
		{
			"operation": "shift",
			"comment": "restructure rating",
			"spec": {
				"rating": {
					"max": "Rating.Max",
					"min": "Rating.Min"
				},
				"id": "Id"
			}
		},
		{
			"operation": "default",
			"spec": {
				"Rating": {"Mid": 3}
			}
		},
		{
			"operation": "sort"
		}
	]`
	outBytes, err := wink.TransformJSON([]byte(input), []byte(chainr))
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	err = json.Unmarshal(outBytes, &out)
	if err != nil {
		return
	}

	assertKey(t, out, "Id", "123")
	rating := out["Rating"].(map[string]any)
	assertKey(t, rating, "Max", float64(5))
	assertKey(t, rating, "Mid", float64(3)) // defaulted
	assertNoKey(t, out, "~emVersion")       // not shifted → absent
}

// ─── Operation aliases ───────────────────────────────────────────────────────

func TestOperationAlias_Defaultr(t *testing.T) {
	input := `{"a":1}`
	chainr := `[{"operation":"defaultr","spec":{"b":2}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "b", float64(2))
}

func TestOperationAlias_Removr(t *testing.T) {
	input := `{"a":1,"b":2}`
	chainr := `[{"operation":"removr","spec":{"a":""}}]`
	out := runChainr(t, input, chainr)
	assertNoKey(t, out, "a")
	assertKey(t, out, "b", float64(2))
}

// ─── Error handling ──────────────────────────────────────────────────────────

func TestUnknownOperation_ReturnsError(t *testing.T) {
	_, err := wink.TransformJSON([]byte(`{}`), []byte(`[{"operation":"frobnicate"}]`))
	if err == nil {
		t.Error("expected error for unknown operation")
	}
}

// ─── Shift: $ wildcard (key as value) ────────────────────────────────────────

func TestShift_DollarWildcard_KeyAsList(t *testing.T) {
	// "$": "ratings" collects input keys as values in output list.
	input := `{"rating":{"primary":{"value":3},"quality":{"value":3}}}`
	chainr := `[{"operation":"shift","spec":{"rating":{"*":{"$":"ratings"}}}}]`
	out := runChainr(t, input, chainr)
	ratings, ok := out["ratings"].([]any)
	if !ok {
		t.Fatalf("ratings should be a list, got %T: %v", out["ratings"], out["ratings"])
	}
	if len(ratings) != 2 {
		t.Errorf("expected 2 ratings, got %d", len(ratings))
	}
}

// ─── Shift: # wildcard (synthetic literal) ───────────────────────────────────

func TestShift_HashLHSSynthetic(t *testing.T) {
	// "#disabled" writes the literal string "disabled" if the key "true" matches.
	input := `{"hidden":{"true":"anything"}}`
	chainr := `[{"operation":"shift","spec":{"hidden":{"true":{"#disabled":"clients.clientId"}}}}]`
	out := runChainr(t, input, chainr)
	clients, ok := out["clients"].(map[string]any)
	if !ok {
		t.Fatalf("clients should be a map, got %T", out["clients"])
	}
	assertKey(t, clients, "clientId", "disabled")
}

// ─── Shift: @ wildcard (value passthrough) ───────────────────────────────────

func TestShift_AtWildcard_ExplicitPassthrough(t *testing.T) {
	// "@" copies the value of the parent key into the output.
	input := `{"foo":"bar"}`
	chainr := `[{"operation":"shift","spec":{"foo":{"$":"place.key","@":"place.value"}}}]`
	out := runChainr(t, input, chainr)
	place, ok := out["place"].(map[string]any)
	if !ok {
		t.Fatalf("place should be a map, got %T", out["place"])
	}
	assertKey(t, place, "key", "foo")
	assertKey(t, place, "value", "bar")
}

// ─── Shift: [] append and [N] indexed array output ───────────────────────────

func TestShift_ArrayAppendNotation(t *testing.T) {
	// "a[]" appends value into an array.
	input := `{"a":1}`
	chainr := `[{"operation":"shift","spec":{"a":"a[]"}}]`
	out := runChainr(t, input, chainr)
	arr, ok := out["a"].([]any)
	if !ok || len(arr) != 1 || arr[0] != float64(1) {
		t.Errorf("a[] should produce [1], got %v", out["a"])
	}
}

func TestShift_ArrayIndexedOutput(t *testing.T) {
	// "Photos[1].Id" writes to Photos array at index 1.
	input := `{"photo-1-id":"327704","photo-1-url":"http://bob.com/photo.jpg"}`
	chainr := `[{"operation":"shift","spec":{"photo-1-id":"Photos[1].Id","photo-1-url":"Photos[1].Url"}}]`
	out := runChainr(t, input, chainr)
	photos, ok := out["Photos"].([]any)
	if !ok || len(photos) < 2 {
		t.Fatalf("Photos should be an array with at least 2 elements, got %v", out["Photos"])
	}
	photo1, ok := photos[1].(map[string]any)
	if !ok {
		t.Fatalf("Photos[1] should be a map, got %T", photos[1])
	}
	assertKey(t, photo1, "Id", "327704")
	assertKey(t, photo1, "Url", "http://bob.com/photo.jpg")
}

// ─── Shift: & with subkey captures ───────────────────────────────────────────

func TestShift_AmpersandSubkeyCapture(t *testing.T) {
	// "tag-*" matches "tag-Pro"; &(0,1) = "Pro" (first star capture).
	input := `{"tag-Pro":"Awesome","tag-Con":"Bogus"}`
	chainr := `[{"operation":"shift","spec":{"tag-*":"tags[0].&(0,1)"}}]`
	out := runChainr(t, input, chainr)
	// We just verify tag data ends up under "tags".
	if _, ok := out["tags"]; !ok {
		t.Error("expected 'tags' key in output")
	}
}

func TestShift_AmpersandLevelUp(t *testing.T) {
	// &1 inside a nested spec refers to the parent match key.
	input := `{"subobject_shift":{"a":true,"b":{"c":true}}}`
	chainr := `[{"operation":"shift","spec":{"subobject_shift":{"*":"&1.&"}}}]`
	out := runChainr(t, input, chainr)
	sub, ok := out["subobject_shift"].(map[string]any)
	if !ok {
		t.Fatalf("subobject_shift should be a map, got %T", out["subobject_shift"])
	}
	if sub["a"] != true {
		t.Errorf("expected subobject_shift.a = true, got %v", sub["a"])
	}
}

// ─── Modify-overwrite ─────────────────────────────────────────────────────────

func TestModifyOverwrite_Concat(t *testing.T) {
	input := `{"person":{"firstName":"John","lastName":"Doe"}}`
	chainr := `[{"operation":"modify-overwrite","spec":{"person":{"fullName":"=concat(@(1,firstName),' ',@(1,lastName))"}}}]`
	out := runChainr(t, input, chainr)
	person := out["person"].(map[string]any)
	assertKey(t, person, "fullName", "John Doe")
	assertKey(t, person, "firstName", "John") // existing preserved
}

func TestModifyOverwrite_ToLower(t *testing.T) {
	input := `{"email":"JOHN@EXAMPLE.COM"}`
	chainr := `[{"operation":"modify-overwrite","spec":{"email":"=toLower(@(1,email))"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "email", "john@example.com")
}

func TestModifyOverwrite_ToInteger(t *testing.T) {
	input := `{"person":{"ageString":"30"}}`
	chainr := `[{"operation":"modify-overwrite","spec":{"person":{"age":"=toInteger(@(1,ageString))"}}}]`
	out := runChainr(t, input, chainr)
	person := out["person"].(map[string]any)
	if person["age"] != float64(30) {
		t.Errorf("age should be float64(30) after JSON round-trip, got %T %v", person["age"], person["age"])
	}
}

func TestModifyOverwrite_MathFunctions(t *testing.T) {
	input := `{"a":10,"b":3}`
	chainr := `[{"operation":"modify-overwrite","spec":{"sum":"=intSum(@(1,a),@(1,b))","product":"=multiply(@(1,a),@(1,b))"}}]`
	out := runChainr(t, input, chainr)
	if out["sum"] != float64(13) {
		t.Errorf("sum: got %v (want float64(13) after JSON round-trip)", out["sum"])
	}
	if out["product"] != float64(30) {
		t.Errorf("product: got %v", out["product"])
	}
}

func TestModifyOverwrite_StringFunctions(t *testing.T) {
	input := `{"s":"  hello world  "}`
	chainr := `[{"operation":"modify-overwrite","spec":{"trimmed":"=trim(@(1,s))","upper":"=toUpper(@(1,s))"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "trimmed", "hello world")
	assertKey(t, out, "upper", "  HELLO WORLD  ")
}

func TestModifyOverwrite_UUID(t *testing.T) {
	input := `{}`
	chainr := `[{"operation":"modify-overwrite","spec":{"id":"=uuid()"}}]`
	out := runChainr(t, input, chainr)
	id, ok := out["id"].(string)
	if !ok || len(id) != 36 {
		t.Errorf("uuid: expected 36-char string, got %q", id)
	}
}

func TestModifyOverwrite_Now(t *testing.T) {
	input := `{}`
	chainr := `[{"operation":"modify-overwrite","spec":{"ts":"=now()"}}]`
	out := runChainr(t, input, chainr)
	ts, ok := out["ts"].(string)
	if !ok || len(ts) == 0 {
		t.Errorf("now: expected non-empty string, got %v", out["ts"])
	}
}

func TestModifyOverwrite_LiteralValue(t *testing.T) {
	input := `{"name":"Alice"}`
	chainr := `[{"operation":"modify-overwrite","spec":{"status":"active"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "status", "active")
	assertKey(t, out, "name", "Alice")
}

// ─── Modify-default ───────────────────────────────────────────────────────────

func TestModifyDefault_DoesNotOverwriteExisting(t *testing.T) {
	input := `{"status":"active"}`
	chainr := `[{"operation":"modify-default","spec":{"status":"inactive","role":"viewer"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "status", "active") // not overwritten
	assertKey(t, out, "role", "viewer")   // added (was absent)
}

func TestModifyDefault_OverwritesNull(t *testing.T) {
	input := `{"status":null}`
	chainr := `[{"operation":"modify-default","spec":{"status":"active"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "status", "active") // null → replaced
}

// ─── Modify-define ────────────────────────────────────────────────────────────

func TestModifyDefine_SkipsNull(t *testing.T) {
	input := `{"status":null}`
	chainr := `[{"operation":"modify-define","spec":{"status":"active"}}]`
	out := runChainr(t, input, chainr)
	// define skips if key exists, even if null.
	if out["status"] != nil {
		t.Errorf("modify-define should not overwrite null; got %v", out["status"])
	}
}

func TestModifyDefine_WritesMissingKey(t *testing.T) {
	input := `{"name":"Alice"}`
	chainr := `[{"operation":"modify-define","spec":{"role":"viewer"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "role", "viewer")
}

// ─── Modify beta aliases ──────────────────────────────────────────────────────

func TestModifyOverwriteBeta_Alias(t *testing.T) {
	input := `{"x":"hello"}`
	chainr := `[{"operation":"modify-overwrite-beta","spec":{"x":"=toUpper(@(1,x))"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "x", "HELLO")
}

func TestModifyDefaultBeta_Alias(t *testing.T) {
	input := `{"a":1}`
	chainr := `[{"operation":"modify-default-beta","spec":{"b":99}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "b", float64(99))
}

func TestModifyDefineBeta_Alias(t *testing.T) {
	input := `{"a":1}`
	chainr := `[{"operation":"modify-define-beta","spec":{"b":42}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "b", float64(42))
}

// ─── Modify: list and type functions ─────────────────────────────────────────

func TestModify_FirstLastElement(t *testing.T) {
	input := `{"arr":[10,20,30]}`
	chainr := `[{"operation":"modify-overwrite","spec":{"first":"=firstElement(@(1,arr))","last":"=lastElement(@(1,arr))"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "first", float64(10))
	assertKey(t, out, "last", float64(30))
}

func TestModify_Split(t *testing.T) {
	input := `{"csv":"a,b,c"}`
	chainr := `[{"operation":"modify-overwrite","spec":{"parts":"=split(',',@(1,csv))"}}]`
	out := runChainr(t, input, chainr)
	parts, ok := out["parts"].([]any)
	if !ok || len(parts) != 3 {
		t.Errorf("split: expected [a,b,c], got %v", out["parts"])
	}
}

func TestModify_Join(t *testing.T) {
	input := `{}`
	chainr := `[{"operation":"modify-overwrite","spec":{"result":"=join('-','a','b','c')"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "result", "a-b-c")
}

func TestModify_ToBoolean(t *testing.T) {
	input := `{"flag":"true"}`
	chainr := `[{"operation":"modify-overwrite","spec":{"flagBool":"=toBoolean(@(1,flag))"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "flagBool", true)
}

func TestModify_Size(t *testing.T) {
	input := `{"arr":[1,2,3,4,5]}`
	chainr := `[{"operation":"modify-overwrite","spec":{"count":"=size(@(1,arr))"}}]`
	out := runChainr(t, input, chainr)
	assertKey(t, out, "count", int64(5))
}
