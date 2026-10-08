package openapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/swaggest/swgui/v5emb"
)

type Binary struct{}

type ErrorResponse struct {
	Status int           `json:"status"`
	Title  string        `json:"title"`
	Errors []ErrorDetail `json:"errors"`
}

type ErrorDetail struct {
	Code   string `json:"code"`
	Field  string `json:"field,omitempty"`
	Detail string `json:"detail"`
}

type Parameter struct {
	Name     string
	In       string
	Required bool
	Type     string
	Format   string
	Example  string
}

type Operation struct {
	ID                  string
	Summary             string
	Description         string
	Tags                []string
	Request             any
	RequestContentType  string
	Response            any
	ResponseContentType string
	ResponseDescription string
	SuccessStatus       int
	FailureStatuses     []int
	Parameters          []Parameter
	Auth                bool
}

type Registry struct {
	mux     *http.ServeMux
	enabled bool
	paths   map[string]map[string]any
}

func NewRegistry(mux *http.ServeMux, enabled bool) *Registry {
	return &Registry{
		mux:     mux,
		enabled: enabled,
		paths:   make(map[string]map[string]any),
	}
}

func (r *Registry) Handle(pattern string, handler http.Handler, operation Operation) {
	r.mux.Handle(pattern, handler)
	r.addOperation(pattern, operation)
}

func (r *Registry) HandleFunc(pattern string, handler http.HandlerFunc, operation Operation) {
	r.Handle(pattern, handler, operation)
}

func (r *Registry) RegisterDocs() {
	if !r.enabled {
		return
	}
	r.mux.Handle("/openapi.json", r.SpecHandler())
	r.mux.Handle("/swagger/", v5emb.New("Anomaly API", "/openapi.json", "/swagger/"))
}

func (r *Registry) SpecHandler() http.Handler {
	spec := r.spec()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(spec)
	})
}

func (r *Registry) addOperation(pattern string, operation Operation) {
	if !r.enabled {
		return
	}
	method, path := splitPattern(pattern)
	if operation.SuccessStatus == 0 {
		operation.SuccessStatus = http.StatusOK
	}
	if operation.ResponseContentType == "" {
		operation.ResponseContentType = "application/json"
	}
	if operation.RequestContentType == "" {
		operation.RequestContentType = "application/json"
	}

	parameters := pathParameters(path)
	for _, parameter := range operation.Parameters {
		parameters = append(parameters, map[string]any{
			"name":     parameter.Name,
			"in":       parameter.In,
			"required": parameter.Required,
			"schema":   parameterSchema(parameter),
		})
	}
	successDescription := operation.ResponseDescription
	if successDescription == "" {
		successDescription = http.StatusText(operation.SuccessStatus)
	}
	responses := map[string]any{
		statusKey(operation.SuccessStatus): responseObject(
			successDescription,
			operation.Response,
			operation.ResponseContentType,
		),
	}
	for _, status := range operation.FailureStatuses {
		responses[statusKey(status)] = responseObject(http.StatusText(status), ErrorResponse{}, "application/json")
	}

	docOperation := map[string]any{
		"responses": responses,
	}
	if operation.ID != "" {
		docOperation["operationId"] = operation.ID
	}
	if operation.Summary != "" {
		docOperation["summary"] = operation.Summary
	}
	if operation.Description != "" {
		docOperation["description"] = operation.Description
	}
	if len(operation.Tags) > 0 {
		docOperation["tags"] = operation.Tags
	}
	if operation.Auth {
		docOperation["security"] = []map[string][]string{{"bearerAuth": []string{}}}
	}
	if len(parameters) > 0 {
		docOperation["parameters"] = parameters
	}
	if operation.Request != nil {
		docOperation["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{
				operation.RequestContentType: map[string]any{
					"schema": schemaFor(reflect.TypeOf(operation.Request)),
				},
			},
		}
	}

	if r.paths[path] == nil {
		r.paths[path] = make(map[string]any)
	}
	r.paths[path][method] = docOperation
}

func (r *Registry) spec() []byte {
	document := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Anomaly API",
			"description": "Account, OTP and media API for Anomaly.",
			"version":     "0.1.0",
		},
		"servers": []map[string]string{{"url": "http://localhost:8080"}},
		"paths":   r.paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
		},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return []byte(`{"openapi":"3.0.3","info":{"title":"Anomaly API","version":"0.1.0"},"paths":{}}`)
	}
	return encoded
}

func splitPattern(pattern string) (string, string) {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		panic("OpenAPI route pattern must include method and path")
	}
	return strings.ToLower(parts[0]), parts[1]
}

func pathParameters(path string) []map[string]any {
	var parameters []map[string]any
	for _, segment := range strings.Split(path, "/") {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		parameters = append(parameters, map[string]any{
			"name":     strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}"),
			"in":       "path",
			"required": true,
			"schema":   map[string]any{"type": "string"},
		})
	}
	return parameters
}

func parameterSchema(parameter Parameter) map[string]any {
	schema := map[string]any{"type": parameter.Type}
	if schema["type"] == "" {
		schema["type"] = "string"
	}
	if parameter.Format != "" {
		schema["format"] = parameter.Format
	}
	if parameter.Example != "" {
		schema["example"] = parameter.Example
	}
	return schema
}

func responseObject(description string, body any, contentType string) map[string]any {
	if description == "" {
		description = http.StatusText(http.StatusOK)
	}
	response := map[string]any{"description": description}
	if body != nil {
		response["content"] = map[string]any{
			contentType: map[string]any{
				"schema": schemaFor(reflect.TypeOf(body)),
			},
		}
	}
	return response
}

func statusKey(status int) string {
	return strconv.Itoa(status)
}

func schemaFor(t reflect.Type) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(Binary{}) {
		return map[string]any{"type": "string", "format": "binary"}
	}
	if t == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}
	}

	switch t.Kind() {
	case reflect.Interface:
		return map[string]any{}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return map[string]any{"type": "integer", "format": "int32"}
	case reflect.Int64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return map[string]any{"type": "integer", "format": "int32"}
	case reflect.Uint64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Float32:
		return map[string]any{"type": "number", "format": "float"}
	case reflect.Float64:
		return map[string]any{"type": "number", "format": "double"}
	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": schemaFor(t.Elem()),
		}
	case reflect.Map:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": schemaFor(t.Elem()),
		}
	case reflect.Struct:
		return structSchema(t)
	default:
		return map[string]any{}
	}
}

func structSchema(t reflect.Type) map[string]any {
	properties := make(map[string]any)
	var required []string
	for index := 0; index < t.NumField(); index++ {
		field := t.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, options := jsonFieldName(field)
		if name == "-" {
			continue
		}
		fieldSchema := schemaFor(field.Type)
		if format := field.Tag.Get("format"); format != "" {
			fieldSchema["format"] = format
		}
		properties[name] = fieldSchema
		if !contains(options, "omitempty") {
			required = append(required, name)
		}
	}
	result := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

func jsonFieldName(field reflect.StructField) (string, []string) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name, nil
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		parts[0] = field.Name
	}
	return parts[0], parts[1:]
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
