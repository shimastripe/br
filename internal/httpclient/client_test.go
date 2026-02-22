package httpclient

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareMethodQueryBodyAutoPost(t *testing.T) {
	method, query, body, err := prepareMethodQueryBody(RequestOptions{
		RawFields: []string{"name=example"},
	}, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("prepare body: %v", err)
	}

	if method != "POST" {
		t.Fatalf("method = %q; want POST", method)
	}
	if len(query) != 0 {
		t.Fatalf("query should be empty, got %v", query)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if decoded["name"] != "example" {
		t.Fatalf("body[name] = %v; want example", decoded["name"])
	}
}

func TestPrepareMethodQueryBodyInputMovesFieldsToQuery(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "body.json")
	if err := os.WriteFile(inputPath, []byte(`{"id":1}`), 0o600); err != nil {
		t.Fatalf("write input file: %v", err)
	}

	method, query, body, err := prepareMethodQueryBody(RequestOptions{
		Method:         "PATCH",
		MethodExplicit: true,
		InputFile:      inputPath,
		RawFields:      []string{"name=example"},
		TypedFields:    []string{"enabled=true", "count=2"},
	}, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("prepare request: %v", err)
	}

	if method != "PATCH" {
		t.Fatalf("method = %q; want PATCH", method)
	}
	if got := string(body); got != `{"id":1}` {
		t.Fatalf("body = %s; want file content", got)
	}
	if query.Get("name") != "example" || query.Get("enabled") != "true" || query.Get("count") != "2" {
		t.Fatalf("unexpected query values: %v", query)
	}
}

func TestPrepareMethodQueryBodyTypedFieldTypes(t *testing.T) {
	method, _, body, err := prepareMethodQueryBody(RequestOptions{
		Method:         "PATCH",
		MethodExplicit: true,
		TypedFields:    []string{"count=2", "ratio=1.5", "enabled=false", "nothing=null"},
	}, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("prepare request: %v", err)
	}
	if method != "PATCH" {
		t.Fatalf("method = %q; want PATCH", method)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if decoded["count"] != float64(2) {
		t.Fatalf("count = %#v; want 2", decoded["count"])
	}
	if decoded["ratio"] != 1.5 {
		t.Fatalf("ratio = %#v; want 1.5", decoded["ratio"])
	}
	if decoded["enabled"] != false {
		t.Fatalf("enabled = %#v; want false", decoded["enabled"])
	}
	if value, ok := decoded["nothing"]; !ok || value != nil {
		t.Fatalf("nothing = %#v (ok=%t); want nil", value, ok)
	}
}

func TestSelectJSONFieldsFromDataList(t *testing.T) {
	payload := []byte(`{"data":[{"slug":"a","name":"n","id":"1"}]}`)

	filtered, err := selectJSONFields(payload, []string{"slug,name"})
	if err != nil {
		t.Fatalf("select fields: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(filtered, &rows); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows length = %d; want 1", len(rows))
	}
	if rows[0]["slug"] != "a" || rows[0]["name"] != "n" {
		t.Fatalf("unexpected row: %#v", rows[0])
	}
	if _, ok := rows[0]["id"]; ok {
		t.Fatalf("unexpected id in filtered row: %#v", rows[0])
	}
}

func TestSelectJSONFieldsFromDataObject(t *testing.T) {
	payload := []byte(`{"data":{"slug":"a","name":"n","id":"1"}}`)

	filtered, err := selectJSONFields(payload, []string{"slug,name"})
	if err != nil {
		t.Fatalf("select fields: %v", err)
	}

	var object map[string]any
	if err := json.Unmarshal(filtered, &object); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}

	if object["slug"] != "a" || object["name"] != "n" {
		t.Fatalf("unexpected object: %#v", object)
	}
	if _, ok := object["id"]; ok {
		t.Fatalf("unexpected id in filtered object: %#v", object)
	}
}

func TestApplyTemplate(t *testing.T) {
	payload := []byte(`[{"slug":"a","name":"n"}]`)

	rendered, err := applyTemplate(`{{range .}}{{.slug}}:{{.name}}{{"\n"}}{{end}}`, payload)
	if err != nil {
		t.Fatalf("apply template: %v", err)
	}
	if strings.TrimSpace(string(rendered)) != "a:n" {
		t.Fatalf("unexpected rendered template: %q", string(rendered))
	}
}
