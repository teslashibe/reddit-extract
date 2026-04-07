package redditextract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const jsonSchemaDraft2020 = "https://json-schema.org/draft/2020-12/schema"

// DynamicSchema represents a runtime JSON schema used for extraction.
type DynamicSchema struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	JSONSchema  map[string]any `json:"json_schema"`
}

// DynamicSchemaFromJSON parses raw JSON schema bytes into a DynamicSchema.
func DynamicSchemaFromJSON(data []byte) (DynamicSchema, error) {
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return DynamicSchema{}, fmt.Errorf("parse schema json: %w", err)
	}
	if len(schema) == 0 {
		return DynamicSchema{}, fmt.Errorf("schema cannot be empty")
	}
	if _, ok := schema["$schema"]; !ok {
		schema["$schema"] = jsonSchemaDraft2020
	}

	out := DynamicSchema{
		JSONSchema: schema,
	}
	if title, ok := schema["title"].(string); ok {
		out.Name = title
	}
	if desc, ok := schema["description"].(string); ok {
		out.Description = desc
	}
	return out, nil
}

// GenerateSchema builds a JSON schema from a typed Go struct.
//
// Supported field kinds include strings, booleans, numbers, integers, slices,
// nested structs, pointers (optional fields), and maps with string keys.
func GenerateSchema[T any]() (DynamicSchema, error) {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return DynamicSchema{}, fmt.Errorf("cannot generate schema for nil type")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	root, err := schemaForType(t, map[reflect.Type]bool{})
	if err != nil {
		return DynamicSchema{}, err
	}

	rootObj, ok := root.(map[string]any)
	if !ok {
		return DynamicSchema{}, fmt.Errorf("root schema must be an object")
	}
	rootObj["$schema"] = jsonSchemaDraft2020
	if t.Name() != "" {
		rootObj["title"] = t.Name()
	}

	return DynamicSchema{
		Name:       t.Name(),
		JSONSchema: rootObj,
	}, nil
}

// DynamicSchemaBuilder builds object schemas programmatically.
type DynamicSchemaBuilder struct {
	name        string
	description string
	properties  map[string]any
	required    []string
}

// NewDynamicSchemaBuilder returns a builder for object-based schemas.
func NewDynamicSchemaBuilder(name string) *DynamicSchemaBuilder {
	return &DynamicSchemaBuilder{
		name:       name,
		properties: make(map[string]any),
	}
}

// WithDescription sets the root schema description.
func (b *DynamicSchemaBuilder) WithDescription(description string) *DynamicSchemaBuilder {
	b.description = description
	return b
}

// AddField adds a raw field schema under the given field name.
func (b *DynamicSchemaBuilder) AddField(name string, fieldSchema map[string]any, required bool) *DynamicSchemaBuilder {
	if strings.TrimSpace(name) == "" || fieldSchema == nil {
		return b
	}
	b.properties[name] = cloneMap(fieldSchema)
	if required {
		b.required = appendUnique(b.required, name)
	}
	return b
}

// AddStringField adds a string field with optional enum constraints.
func (b *DynamicSchemaBuilder) AddStringField(name, description string, required bool, enum ...string) *DynamicSchemaBuilder {
	field := map[string]any{
		"type": "string",
	}
	if description != "" {
		field["description"] = description
	}
	if len(enum) > 0 {
		field["enum"] = enum
	}
	return b.AddField(name, field, required)
}

// AddNumberField adds a number field.
func (b *DynamicSchemaBuilder) AddNumberField(name, description string, required bool) *DynamicSchemaBuilder {
	field := map[string]any{
		"type": "number",
	}
	if description != "" {
		field["description"] = description
	}
	return b.AddField(name, field, required)
}

// AddBooleanField adds a boolean field.
func (b *DynamicSchemaBuilder) AddBooleanField(name, description string, required bool) *DynamicSchemaBuilder {
	field := map[string]any{
		"type": "boolean",
	}
	if description != "" {
		field["description"] = description
	}
	return b.AddField(name, field, required)
}

// AddArrayField adds an array field with a caller-defined item schema.
func (b *DynamicSchemaBuilder) AddArrayField(name, description string, required bool, itemSchema map[string]any) *DynamicSchemaBuilder {
	field := map[string]any{
		"type":  "array",
		"items": cloneMap(itemSchema),
	}
	if description != "" {
		field["description"] = description
	}
	return b.AddField(name, field, required)
}

// Build returns an immutable DynamicSchema value from the builder.
func (b *DynamicSchemaBuilder) Build() DynamicSchema {
	root := map[string]any{
		"$schema":              jsonSchemaDraft2020,
		"type":                 "object",
		"properties":           cloneMap(b.properties),
		"additionalProperties": false,
	}
	if b.name != "" {
		root["title"] = b.name
	}
	if b.description != "" {
		root["description"] = b.description
	}
	if len(b.required) > 0 {
		root["required"] = append([]string(nil), b.required...)
	}

	return DynamicSchema{
		Name:        b.name,
		Description: b.description,
		JSONSchema:  root,
	}
}

func schemaForType(t reflect.Type, seen map[reflect.Type]bool) (any, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.PkgPath() == "time" && t.Name() == "Time" {
		return map[string]any{
			"type":   "string",
			"format": "date-time",
		}, nil
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Slice, reflect.Array:
		itemSchema, err := schemaForType(t.Elem(), seen)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":  "array",
			"items": itemSchema,
		}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported map key type %s (must be string)", t.Key())
		}
		additional := any(true)
		if t.Elem().Kind() != reflect.Interface {
			elemSchema, err := schemaForType(t.Elem(), seen)
			if err != nil {
				return nil, err
			}
			additional = elemSchema
		}
		return map[string]any{
			"type":                 "object",
			"additionalProperties": additional,
		}, nil
	case reflect.Struct:
		if seen[t] {
			return nil, fmt.Errorf("recursive schema not supported for type %s", t.String())
		}
		seen[t] = true
		defer delete(seen, t)

		properties := make(map[string]any)
		required := make([]string, 0, t.NumField())

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}

			name, omitempty, skip := parseJSONTag(field)
			if skip {
				continue
			}

			fieldType := field.Type
			optional := omitempty || fieldType.Kind() == reflect.Pointer
			if fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}

			fieldSchemaAny, err := schemaForType(fieldType, seen)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}

			fieldSchema, ok := fieldSchemaAny.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("field %s: schema must be object", field.Name)
			}

			fieldSchema = cloneMap(fieldSchema)
			if desc := strings.TrimSpace(field.Tag.Get("desc")); desc != "" {
				fieldSchema["description"] = desc
			}
			if enumVals := parseEnumTag(field.Tag.Get("enum")); len(enumVals) > 0 {
				fieldSchema["enum"] = enumVals
			}

			properties[name] = fieldSchema
			if !optional {
				required = append(required, name)
			}
		}

		obj := map[string]any{
			"type":                 "object",
			"properties":           properties,
			"additionalProperties": false,
		}
		if len(required) > 0 {
			obj["required"] = required
		}
		return obj, nil
	case reflect.Interface:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported kind: %s", t.Kind())
	}
}

func parseJSONTag(field reflect.StructField) (name string, omitempty bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return field.Name, false, false
	}

	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = field.Name
	}
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

func parseEnumTag(tag string) []string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	parts := strings.Split(tag, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		val := strings.TrimSpace(part)
		if val != "" {
			out = append(out, val)
		}
	}
	return out
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneMap(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = cloneValue(item)
		}
		return out
	case []string:
		out := make([]string, len(x))
		copy(out, x)
		return out
	default:
		return v
	}
}
