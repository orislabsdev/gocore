package openapi

import (
	"fmt"
	"strings"

	"github.com/orislabsdev/gocore/router"
)

// Document represents an OpenAPI 3.0 specification.
type Document struct {
	OpenAPI    string               `json:"openapi"`
	Info       Info                 `json:"info"`
	Paths      map[string]*PathItem `json:"paths"`
	Components *Components          `json:"components,omitempty"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Head    *Operation `json:"head,omitempty"`
}

type Operation struct {
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	OperationID string              `json:"operationId,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"` // "query", "header", "path" or "cookie"
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required"`
	Schema      *Schema `json:"schema"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Content     map[string]MediaType `json:"content"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema"`
}

type Components struct {
	Schemas map[string]*Schema `json:"schemas,omitempty"`
}

// Generate builds an OpenAPI 3.0 Document from the provided routes and info.
func Generate(info Info, routes []router.RouteInfo) *Document {
	doc := &Document{
		OpenAPI:    "3.0.3",
		Info:       info,
		Paths:      make(map[string]*PathItem),
		Components: &Components{Schemas: make(map[string]*Schema)},
	}

	for _, rt := range routes {
		path, params := formatPathAndExtractParams(rt.Pattern)

		item, ok := doc.Paths[path]
		if !ok {
			item = &PathItem{}
			doc.Paths[path] = item
		}

		op := &Operation{
			Summary:     rt.Summary,
			Description: rt.Description,
			Tags:        rt.Tags,
			OperationID: rt.Name,
			Responses:   make(map[string]Response),
		}

		// Add path parameters
		for _, paramName := range params {
			op.Parameters = append(op.Parameters, Parameter{
				Name:     paramName,
				In:       "path",
				Required: true,
				Schema:   &Schema{Type: "string"},
			})
		}

		// Add Request Body
		if rt.RequestBody != nil {
			op.RequestBody = &RequestBody{
				Content: map[string]MediaType{
					"application/json": {
						Schema: GenerateSchema(rt.RequestBody, doc.Components),
					},
				},
				Required: true,
			}
		}

		// Add Responses
		// If no responses defined, provide a default 200 (or 204 if no body... but 200 is fine)
		if len(rt.Responses) == 0 {
			op.Responses["200"] = Response{Description: "OK"}
		} else {
			for _, resp := range rt.Responses {
				codeStr := fmt.Sprintf("%d", resp.Code)
				r := Response{
					Description: resp.Description,
				}
				if resp.Example != nil {
					r.Content = map[string]MediaType{
						"application/json": {
							Schema: GenerateSchema(resp.Example, doc.Components),
						},
					}
				}
				op.Responses[codeStr] = r
			}
		}

		switch rt.Method {
		case "GET":
			item.Get = op
		case "POST":
			item.Post = op
		case "PUT":
			item.Put = op
		case "PATCH":
			item.Patch = op
		case "DELETE":
			item.Delete = op
		case "OPTIONS":
			item.Options = op
		case "HEAD":
			item.Head = op
		}
	}

	return doc
}

// formatPathAndExtractParams converts "/users/:id" to "/users/{id}" and returns ["id"].
func formatPathAndExtractParams(pattern string) (string, []string) {
	parts := strings.Split(pattern, "/")
	var params []string
	var out []string

	for _, p := range parts {
		if strings.HasPrefix(p, ":") {
			paramName := strings.TrimPrefix(p, ":")
			params = append(params, paramName)
			out = append(out, "{"+paramName+"}")
		} else if strings.HasPrefix(p, "*") {
			paramName := strings.TrimPrefix(p, "*")
			params = append(params, paramName)
			out = append(out, "{"+paramName+"}")
		} else {
			out = append(out, p)
		}
	}

	return strings.Join(out, "/"), params
}
