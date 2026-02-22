package main

import (
	"path/filepath"
	"testing"
)

func TestDeriveSubcommandName(t *testing.T) {
	cases := []struct {
		tag   string
		opID  string
		want  string
		title string
	}{
		{title: "plural prefix", tag: "addons", opID: "addons-list", want: "list"},
		{title: "singular prefix", tag: "addons", opID: "addon-list-by-app", want: "list-by-app"},
		{title: "no prefix", tag: "user", opID: "machine-type-update", want: "machine-type-update"},
	}

	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			got := deriveSubcommandName(tc.tag, tc.opID)
			if got != tc.want {
				t.Fatalf("deriveSubcommandName(%q, %q) = %q; want %q", tc.tag, tc.opID, got, tc.want)
			}
		})
	}
}

func TestBuildOperationsFromSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "spec", "bitrise-swagger.json")
	spec, err := loadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	ops, err := buildOperations(spec)
	if err != nil {
		t.Fatalf("build operations: %v", err)
	}

	if got, want := len(ops), 115; got != want {
		t.Fatalf("operation count = %d; want %d", got, want)
	}

	seenByTag := map[string]map[string]bool{}
	foundAddonsList := false
	foundAddonsShow := false
	for _, op := range ops {
		if op.OperationID == "secret-upsert" || op.OperationID == "secret-value-get" {
			t.Fatalf("deprecated operation %q should not be generated", op.OperationID)
		}
		if op.Method == "GET" && !op.SupportsJSON {
			t.Fatalf("GET operation %q should support --fields", op.OperationID)
		}
		if op.Method != "GET" && op.SupportsJSON {
			t.Fatalf("non-GET operation %q should not support --fields", op.OperationID)
		}
		if seenByTag[op.Tag] == nil {
			seenByTag[op.Tag] = map[string]bool{}
		}
		if seenByTag[op.Tag][op.Name] {
			t.Fatalf("duplicate command name %q under tag %q", op.Name, op.Tag)
		}
		seenByTag[op.Tag][op.Name] = true

		if op.OperationID == "addons-list" {
			foundAddonsList = true
			if !containsField(op.JSONFields, "id") {
				t.Fatalf("addons-list JSON fields should include id, got %v", op.JSONFields)
			}
		}
		if op.OperationID == "addons-show" {
			foundAddonsShow = true
			if !containsField(op.JSONFields, "id") {
				t.Fatalf("addons-show JSON fields should include id, got %v", op.JSONFields)
			}
		}
	}

	if !foundAddonsList {
		t.Fatal("addons-list operation not found")
	}
	if !foundAddonsShow {
		t.Fatal("addons-show operation not found")
	}
}

func containsField(fields []string, expected string) bool {
	for _, field := range fields {
		if field == expected {
			return true
		}
	}
	return false
}
