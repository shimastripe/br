package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultSpecPath = "spec/bitrise-swagger.json"
	defaultOutPath  = "internal/cli/generated/commands_gen.go"
)

type swaggerSpec struct {
	Paths       map[string]map[string]swaggerOperation `json:"paths"`
	Definitions map[string]swaggerSchema               `json:"definitions"`
}

type swaggerOperation struct {
	OperationID string                     `json:"operationId"`
	Summary     string                     `json:"summary"`
	Description string                     `json:"description"`
	Tags        []string                   `json:"tags"`
	Deprecated  bool                       `json:"deprecated"`
	Parameters  []swaggerParam             `json:"parameters"`
	Responses   map[string]swaggerResponse `json:"responses"`
}

type swaggerResponse struct {
	Description string         `json:"description"`
	Schema      *swaggerSchema `json:"schema"`
}

type swaggerSchema struct {
	Ref        string                   `json:"$ref"`
	Type       string                   `json:"type"`
	Properties map[string]swaggerSchema `json:"properties"`
	Items      *swaggerSchema           `json:"items"`
	AllOf      []swaggerSchema          `json:"allOf"`
}

type swaggerParam struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Required    bool            `json:"required"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

type operation struct {
	Tag          string
	Name         string
	OperationID  string
	Method       string
	Path         string
	Summary      string
	Description  string
	BodyRequired bool
	SupportsJSON bool
	JSONFields   []string
	Params       []param
}

type param struct {
	Name        string
	In          string
	Required    bool
	Type        string
	Description string
}

func main() {
	specPath := flag.String("spec", defaultSpecPath, "path to Swagger spec JSON")
	outPath := flag.String("out", defaultOutPath, "output path for generated commands")
	flag.Parse()

	spec, err := loadSpec(*specPath)
	if err != nil {
		panic(err)
	}

	operations, err := buildOperations(spec)
	if err != nil {
		panic(err)
	}

	source, err := renderGeneratedFile(operations)
	if err != nil {
		panic(err)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*outPath, source, 0o644); err != nil {
		panic(err)
	}
}

func loadSpec(path string) (*swaggerSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	var spec swaggerSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	if len(spec.Paths) == 0 {
		return nil, fmt.Errorf("spec contains no paths")
	}

	return &spec, nil
}

func buildOperations(spec *swaggerSpec) ([]operation, error) {
	paths := make([]string, 0, len(spec.Paths))
	for path := range spec.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	operations := make([]operation, 0, 128)
	for _, path := range paths {
		methodsMap := spec.Paths[path]
		methods := make([]string, 0, len(methodsMap))
		for method := range methodsMap {
			methods = append(methods, method)
		}
		sort.Strings(methods)

		for _, method := range methods {
			op := methodsMap[method]
			if op.Deprecated {
				continue
			}
			if len(op.Tags) == 0 {
				continue
			}

			opID := strings.TrimSpace(op.OperationID)
			if opID == "" {
				opID = fallbackOperationID(strings.ToUpper(method), path)
			}

			methodUpper := strings.ToUpper(method)
			supportsJSON := methodUpper == "GET"
			jsonFields := []string(nil)
			if supportsJSON {
				jsonFields = extractJSONFields(spec, op)
			}

			for _, tag := range op.Tags {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					continue
				}
				params := make([]param, 0, len(op.Parameters))
				bodyRequired := false
				for _, p := range op.Parameters {
					pt := strings.TrimSpace(p.Type)
					if pt == "" && p.In == "body" {
						pt = "object"
					}
					if p.In == "body" && p.Required {
						bodyRequired = true
					}
					params = append(params, param{
						Name:        p.Name,
						In:          p.In,
						Required:    p.Required,
						Type:        pt,
						Description: p.Description,
					})
				}
				sort.Slice(params, func(i, j int) bool {
					if params[i].In == params[j].In {
						return params[i].Name < params[j].Name
					}
					return params[i].In < params[j].In
				})

				operations = append(operations, operation{
					Tag:          tag,
					Name:         deriveSubcommandName(tag, opID),
					OperationID:  opID,
					Method:       methodUpper,
					Path:         path,
					Summary:      strings.TrimSpace(op.Summary),
					Description:  strings.TrimSpace(op.Description),
					BodyRequired: bodyRequired,
					SupportsJSON: supportsJSON,
					JSONFields:   jsonFields,
					Params:       params,
				})
			}
		}
	}

	if len(operations) == 0 {
		return nil, fmt.Errorf("no operations generated")
	}

	assignUniqueNames(operations)

	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Tag != operations[j].Tag {
			return operations[i].Tag < operations[j].Tag
		}
		if operations[i].Name != operations[j].Name {
			return operations[i].Name < operations[j].Name
		}
		return operations[i].OperationID < operations[j].OperationID
	})

	return operations, nil
}

func extractJSONFields(spec *swaggerSpec, op swaggerOperation) []string {
	schema, ok := findSuccessSchema(op.Responses)
	if !ok {
		return nil
	}

	projected := projectResponseSchema(spec, *schema)
	fields := topLevelObjectFields(spec, projected)
	if len(fields) > 0 {
		return fields
	}

	// Fallback to root object fields when projection does not expose field names.
	return topLevelObjectFields(spec, *schema)
}

func findSuccessSchema(responses map[string]swaggerResponse) (*swaggerSchema, bool) {
	if len(responses) == 0 {
		return nil, false
	}

	statuses := make([]string, 0, len(responses))
	for status := range responses {
		if strings.HasPrefix(strings.TrimSpace(status), "2") {
			statuses = append(statuses, status)
		}
	}
	sort.Strings(statuses)

	for _, status := range statuses {
		response := responses[status]
		if response.Schema != nil {
			return response.Schema, true
		}
	}
	return nil, false
}

func projectResponseSchema(spec *swaggerSpec, schema swaggerSchema) swaggerSchema {
	resolved := resolveSchema(spec, schema, map[string]bool{})
	if isObjectSchema(resolved) {
		if dataSchema, ok := resolved.Properties["data"]; ok {
			dataResolved := resolveSchema(spec, dataSchema, map[string]bool{})
			if isArraySchema(dataResolved) && dataResolved.Items != nil {
				itemResolved := resolveSchema(spec, *dataResolved.Items, map[string]bool{})
				if isObjectSchema(itemResolved) {
					return itemResolved
				}
			}
			if isObjectSchema(dataResolved) {
				return dataResolved
			}
		}
		return resolved
	}

	if isArraySchema(resolved) && resolved.Items != nil {
		itemResolved := resolveSchema(spec, *resolved.Items, map[string]bool{})
		if isObjectSchema(itemResolved) {
			return itemResolved
		}
	}

	return resolved
}

func topLevelObjectFields(spec *swaggerSpec, schema swaggerSchema) []string {
	resolved := resolveSchema(spec, schema, map[string]bool{})
	if !isObjectSchema(resolved) {
		return nil
	}

	fields := make([]string, 0, len(resolved.Properties))
	for key := range resolved.Properties {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

func resolveSchema(spec *swaggerSpec, schema swaggerSchema, seen map[string]bool) swaggerSchema {
	if seen == nil {
		seen = map[string]bool{}
	}
	resolved := schema

	ref := strings.TrimSpace(resolved.Ref)
	if ref != "" {
		defName := strings.TrimPrefix(ref, "#/definitions/")
		if defName == "" {
			return swaggerSchema{}
		}
		if seen[defName] {
			return swaggerSchema{}
		}
		definition, ok := spec.Definitions[defName]
		if !ok {
			return swaggerSchema{}
		}
		seen[defName] = true
		defer delete(seen, defName)
		resolved = resolveSchema(spec, definition, seen)
	}

	if len(resolved.AllOf) > 0 {
		merged := swaggerSchema{}
		for _, part := range resolved.AllOf {
			merged = mergeSchemas(merged, resolveSchema(spec, part, seen))
		}
		own := swaggerSchema{
			Type:       resolved.Type,
			Properties: resolved.Properties,
			Items:      resolved.Items,
		}
		resolved = mergeSchemas(merged, own)
	}

	return resolved
}

func mergeSchemas(base swaggerSchema, add swaggerSchema) swaggerSchema {
	merged := swaggerSchema{
		Type:       base.Type,
		Properties: map[string]swaggerSchema{},
		Items:      base.Items,
	}

	for key, value := range base.Properties {
		merged.Properties[key] = value
	}
	for key, value := range add.Properties {
		merged.Properties[key] = value
	}

	if strings.TrimSpace(add.Type) != "" {
		merged.Type = add.Type
	}
	if add.Items != nil {
		merged.Items = add.Items
	}
	if len(merged.Properties) == 0 {
		merged.Properties = nil
	}

	return merged
}

func isObjectSchema(schema swaggerSchema) bool {
	return strings.EqualFold(schema.Type, "object") || len(schema.Properties) > 0
}

func isArraySchema(schema swaggerSchema) bool {
	return strings.EqualFold(schema.Type, "array") || schema.Items != nil
}

func assignUniqueNames(operations []operation) {
	used := map[string]map[string]int{}
	for i := range operations {
		tag := operations[i].Tag
		if used[tag] == nil {
			used[tag] = map[string]int{}
		}

		name := operations[i].Name
		if name == "" {
			name = normalizeToken(operations[i].OperationID)
		}
		if name == "" {
			name = normalizeToken(strings.ToLower(operations[i].Method))
		}

		if _, ok := used[tag][name]; ok {
			candidate := normalizeToken(operations[i].OperationID)
			if candidate == "" {
				candidate = name
			}
			if _, exists := used[tag][candidate]; exists {
				idx := used[tag][candidate] + 1
				candidate = fmt.Sprintf("%s-%d", candidate, idx)
			}
			name = candidate
		}

		used[tag][name]++
		operations[i].Name = name
	}
}

func fallbackOperationID(method string, path string) string {
	token := strings.Trim(path, "/")
	token = strings.ReplaceAll(token, "/", "-")
	token = strings.ReplaceAll(token, "{", "")
	token = strings.ReplaceAll(token, "}", "")
	token = strings.ReplaceAll(token, "_", "-")
	token = normalizeToken(token)
	if token == "" {
		token = "endpoint"
	}
	return strings.ToLower(method) + "-" + token
}

func deriveSubcommandName(tag string, operationID string) string {
	op := normalizeToken(operationID)
	tagToken := normalizeToken(tag)
	singularTag := singular(tagToken)

	prefixes := []string{tagToken + "-", singularTag + "-"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(op, prefix) {
			trimmed := strings.TrimPrefix(op, prefix)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return op
}

func normalizeToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "/", "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	value = strings.Trim(value, "-")
	return value
}

func singular(tag string) string {
	if strings.HasSuffix(tag, "ies") {
		return strings.TrimSuffix(tag, "ies") + "y"
	}
	if strings.HasSuffix(tag, "s") {
		return strings.TrimSuffix(tag, "s")
	}
	return tag
}

func renderGeneratedFile(operations []operation) ([]byte, error) {
	grouped := map[string][]operation{}
	for _, op := range operations {
		grouped[op.Tag] = append(grouped[op.Tag], op)
	}

	tags := make([]string, 0, len(grouped))
	for tag := range grouped {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	var buf bytes.Buffer
	buf.WriteString("// Code generated by go run ./internal/gen/openapi_gen.go; DO NOT EDIT.\n")
	buf.WriteString("\n")
	buf.WriteString("package generated\n\n")
	buf.WriteString(fmt.Sprintf("const NonDeprecatedOperationCount = %d\n\n", len(operations)))
	buf.WriteString("var Tags = []TagSpec{\n")

	for _, tag := range tags {
		ops := grouped[tag]
		sort.Slice(ops, func(i, j int) bool {
			if ops[i].Name != ops[j].Name {
				return ops[i].Name < ops[j].Name
			}
			return ops[i].OperationID < ops[j].OperationID
		})

		buf.WriteString("\t{\n")
		buf.WriteString(fmt.Sprintf("\t\tName: %q,\n", tag))
		buf.WriteString("\t\tOperations: []OperationSpec{\n")

		for _, op := range ops {
			buf.WriteString("\t\t\t{\n")
			buf.WriteString(fmt.Sprintf("\t\t\t\tName: %q,\n", op.Name))
			buf.WriteString(fmt.Sprintf("\t\t\t\tOperationID: %q,\n", op.OperationID))
			buf.WriteString(fmt.Sprintf("\t\t\t\tMethod: %q,\n", op.Method))
			buf.WriteString(fmt.Sprintf("\t\t\t\tPath: %q,\n", op.Path))
			buf.WriteString(fmt.Sprintf("\t\t\t\tSummary: %q,\n", op.Summary))
			buf.WriteString(fmt.Sprintf("\t\t\t\tDescription: %q,\n", op.Description))
			buf.WriteString(fmt.Sprintf("\t\t\t\tBodyRequired: %t,\n", op.BodyRequired))
			buf.WriteString(fmt.Sprintf("\t\t\t\tSupportsJSON: %t,\n", op.SupportsJSON))
			buf.WriteString("\t\t\t\tJSONFields: []string{")
			for i, field := range op.JSONFields {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(fmt.Sprintf("%q", field))
			}
			buf.WriteString("},\n")
			buf.WriteString("\t\t\t\tParams: []ParamSpec{\n")
			for _, p := range op.Params {
				buf.WriteString("\t\t\t\t\t{\n")
				buf.WriteString(fmt.Sprintf("\t\t\t\t\t\tName: %q,\n", p.Name))
				buf.WriteString(fmt.Sprintf("\t\t\t\t\t\tIn: %q,\n", p.In))
				buf.WriteString(fmt.Sprintf("\t\t\t\t\t\tRequired: %t,\n", p.Required))
				buf.WriteString(fmt.Sprintf("\t\t\t\t\t\tType: %q,\n", p.Type))
				buf.WriteString(fmt.Sprintf("\t\t\t\t\t\tDescription: %q,\n", p.Description))
				buf.WriteString("\t\t\t\t\t},\n")
			}
			buf.WriteString("\t\t\t\t},\n")
			buf.WriteString("\t\t\t},\n")
		}

		buf.WriteString("\t\t},\n")
		buf.WriteString("\t},\n")
	}

	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated file: %w", err)
	}
	return formatted, nil
}
