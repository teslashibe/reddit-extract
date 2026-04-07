package redditextract

import (
	"encoding/json"
	"testing"
)

type schemaNested struct {
	Label string `json:"label" desc:"Nested label"`
}

type schemaFixture struct {
	Topic      string         `json:"topic" desc:"Main topic" enum:"trend,question,review"`
	Score      int            `json:"score"`
	Confidence *float64       `json:"confidence,omitempty"`
	Tags       []string       `json:"tags"`
	Nested     schemaNested   `json:"nested"`
	Meta       map[string]any `json:"meta,omitempty"`
}

func TestGenerateSchemaFromStruct(t *testing.T) {
	schema, err := GenerateSchema[schemaFixture]()
	if err != nil {
		t.Fatalf("GenerateSchema() error = %v", err)
	}

	root := schema.JSONSchema
	if root["$schema"] == "" {
		t.Fatalf("missing $schema")
	}
	if root["type"] != "object" {
		t.Fatalf("root type = %v, want object", root["type"])
	}

	properties, ok := root["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type mismatch")
	}
	topic := properties["topic"].(map[string]any)
	if topic["description"] != "Main topic" {
		t.Fatalf("topic description mismatch: %v", topic["description"])
	}
	enumVals, ok := topic["enum"].([]string)
	if !ok {
		t.Fatalf("topic enum type mismatch")
	}
	if len(enumVals) != 3 || enumVals[0] != "trend" {
		t.Fatalf("topic enum mismatch: %+v", enumVals)
	}

	required, ok := root["required"].([]string)
	if !ok {
		t.Fatalf("required type mismatch")
	}
	contains := func(items []string, target string) bool {
		for _, item := range items {
			if item == target {
				return true
			}
		}
		return false
	}
	if !contains(required, "topic") || !contains(required, "score") || !contains(required, "tags") {
		t.Fatalf("required missing expected fields: %+v", required)
	}
	if contains(required, "confidence") {
		t.Fatalf("confidence should not be required: %+v", required)
	}
}

func TestDynamicSchemaFromJSON(t *testing.T) {
	raw := []byte(`{
		"title":"TrendSchema",
		"type":"object",
		"properties":{
			"trend":{"type":"string"}
		},
		"required":["trend"]
	}`)

	schema, err := DynamicSchemaFromJSON(raw)
	if err != nil {
		t.Fatalf("DynamicSchemaFromJSON() error = %v", err)
	}
	if schema.Name != "TrendSchema" {
		t.Fatalf("schema.Name = %q, want TrendSchema", schema.Name)
	}
	if schema.JSONSchema["$schema"] == nil {
		t.Fatal("expected $schema to be added")
	}
}

func TestDynamicSchemaBuilder(t *testing.T) {
	schema := NewDynamicSchemaBuilder("RuntimeSchema").
		WithDescription("Runtime extraction").
		AddStringField("sentiment", "Overall sentiment", true, "positive", "neutral", "negative").
		AddNumberField("confidence", "Model confidence", false).
		Build()

	if schema.Name != "RuntimeSchema" {
		t.Fatalf("schema.Name = %q", schema.Name)
	}
	if schema.JSONSchema["type"] != "object" {
		t.Fatalf("schema type = %v", schema.JSONSchema["type"])
	}

	data, err := json.Marshal(schema.JSONSchema)
	if err != nil {
		t.Fatalf("Marshal(schema) error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("schema JSON is empty")
	}
}
