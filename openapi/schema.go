package openapi

import (
	"reflect"
	"strings"
)

// Schema represents a JSON Schema object for OpenAPI 3.0.
type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Description          string             `json:"description,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
}

// GenerateSchema uses reflection to build an OpenAPI Schema object.
func GenerateSchema(v any, components *Components) *Schema {
	if v == nil {
		return &Schema{Type: "object"}
	}
	t := reflect.TypeOf(v)
	return generateSchemaFromType(t, components)
}

func generateSchemaFromType(t reflect.Type, components *Components) *Schema {
	if t == nil {
		return &Schema{Type: "object"}
	}

	// Unroll pointers
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		// To avoid infinite loops and massive inline schemas, we register structs
		// in the components map and return a $ref.
		name := t.Name()
		if name == "" {
			return buildStructSchema(t, components) // fallback for anonymous inline structs
		}

		if components.Schemas == nil {
			components.Schemas = make(map[string]*Schema)
		}

		if _, exists := components.Schemas[name]; !exists {
			// Pre-register to break cycles
			components.Schemas[name] = &Schema{Type: "object"}
			components.Schemas[name] = buildStructSchema(t, components)
		}
		return &Schema{Ref: "#/components/schemas/" + name}

	case reflect.Slice, reflect.Array:
		items := generateSchemaFromType(t.Elem(), components)
		return &Schema{Type: "array", Items: items}

	case reflect.Map:
		elem := generateSchemaFromType(t.Elem(), components)
		return &Schema{Type: "object", AdditionalProperties: elem}

	case reflect.String:
		return &Schema{Type: "string"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}

	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}

	case reflect.Bool:
		return &Schema{Type: "boolean"}

	default:
		// Fallback for interface{}, chan, func, etc.
		return &Schema{Type: "object"}
	}
}

func buildStructSchema(t reflect.Type, components *Components) *Schema {
	s := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name := field.Name
		parts := strings.Split(jsonTag, ",")
		if len(parts) > 0 && parts[0] != "" {
			name = parts[0]
		}

		// Handle embedded fields
		if field.Anonymous && (jsonTag == "" || parts[0] == "") {
			embSchema := generateSchemaFromType(field.Type, components)
			if embSchema.Ref != "" {
				// If it's a ref, we can't easily merge without full resolution in this simple builder.
				// We map it to the struct name.
				name = field.Type.Name()
				s.Properties[name] = embSchema
			} else if embSchema.Properties != nil {
				// Merge properties directly
				for k, v := range embSchema.Properties {
					s.Properties[k] = v
				}
			}
			continue
		}

		propSchema := generateSchemaFromType(field.Type, components)
		s.Properties[name] = propSchema
	}

	return s
}
