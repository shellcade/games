package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileSchema compiles the declared odds-variant JSON Schema (test-only
// dependency; the wasm build never compiles schemas — the platform does).
func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(oddsVariantSchema))
	if err != nil {
		t.Fatalf("declared schema is not JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("odds-variant.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("odds-variant.json")
	if err != nil {
		t.Fatalf("declared schema does not compile: %v", err)
	}
	return s
}

func validate(t *testing.T, s *jsonschema.Schema, doc string) error {
	t.Helper()
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("test doc is not JSON: %v", err)
	}
	return s.Validate(inst)
}

// TestDeclaredDefaultMatchesCompiledDefault pins the declared Default against
// the compiled-in default: the JSON document must parse through the same
// parser the room uses and compile to an identical variant, so the admin
// screen's "unset → default" can never drift from what the machine runs.
func TestDeclaredDefaultMatchesCompiledDefault(t *testing.T) {
	declared, err := parseVariant([]byte(defaultVariantJSON))
	if err != nil {
		t.Fatalf("declared default does not parse: %v", err)
	}
	compiled := defaultVariant()
	if declared.name != compiled.name {
		t.Fatalf("name: declared %q, compiled %q", declared.name, compiled.name)
	}
	if !reflect.DeepEqual(declared.strip, compiled.strip) {
		t.Fatalf("strip mismatch:\ndeclared %v\ncompiled %v", declared.strip, compiled.strip)
	}
	if !reflect.DeepEqual(declared.pays, compiled.pays) {
		t.Fatalf("paytable mismatch:\ndeclared %v\ncompiled %v", declared.pays, compiled.pays)
	}
}

// TestDeclaredSchemaAcceptsDefaultRejectsGarbage pins the declared schema:
// the declared default passes, and malformed samples (wrong symbol, negative
// multiplier, missing name, empty paytable) fail.
func TestDeclaredSchemaAcceptsDefaultRejectsGarbage(t *testing.T) {
	s := compileSchema(t)
	if err := validate(t, s, defaultVariantJSON); err != nil {
		t.Fatalf("schema rejects the declared default: %v", err)
	}
	bad := map[string]string{
		"unknown symbol":      `{"name":"x","weights":{"Q":1},"paytable":[{"faces":"7","multiplier":1}]}`,
		"negative multiplier": `{"name":"x","weights":{"7":1},"paytable":[{"faces":"7","multiplier":-5}]}`,
		"missing name":        `{"weights":{"7":1},"paytable":[{"faces":"7","multiplier":1}]}`,
		"empty paytable":      `{"name":"x","weights":{"7":1},"paytable":[]}`,
		"non-integer weight":  `{"name":"x","weights":{"7":1.5},"paytable":[{"faces":"7","multiplier":1}]}`,
	}
	for name, doc := range bad {
		if err := validate(t, s, doc); err == nil {
			t.Errorf("%s: schema accepted %s", name, doc)
		}
	}
}

// TestMetaDeclaresOddsVariantSpec pins the declared spec wiring: the key the
// room reads is the key the spec declares, typed json with both documents.
func TestMetaDeclaresOddsVariantSpec(t *testing.T) {
	m := Game{}.Meta()
	if len(m.Config) != 1 {
		t.Fatalf("want 1 config spec, got %d", len(m.Config))
	}
	cs := m.Config[0]
	if cs.Key != configKey || cs.Default != defaultVariantJSON || cs.Schema != oddsVariantSchema {
		t.Fatalf("spec wiring mismatch: %+v", cs)
	}
}
