package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shimastripe/br/internal/auth"
	"github.com/zalando/go-keyring"
)

type memoryKeyring struct {
	data map[string]string
}

func newMemoryKeyring() *memoryKeyring {
	return &memoryKeyring{data: map[string]string{}}
}

func (m *memoryKeyring) key(service string, user string) string {
	return service + "|" + user
}

func (m *memoryKeyring) Set(service string, user string, password string) error {
	m.data[m.key(service, user)] = password
	return nil
}

func (m *memoryKeyring) Get(service string, user string) (string, error) {
	value, ok := m.data[m.key(service, user)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (m *memoryKeyring) Delete(service string, user string) error {
	delete(m.data, m.key(service, user))
	return nil
}

func newTestStore(t *testing.T) *auth.Store {
	t.Helper()
	store, err := auth.NewStoreWithOptions(auth.StoreOptions{
		Keyring:   newMemoryKeyring(),
		HostsPath: filepath.Join(t.TempDir(), "hosts.yml"),
	})
	if err != nil {
		t.Fatalf("create test auth store: %v", err)
	}
	return store
}

func executeCLI(t *testing.T, store *auth.Store, client *http.Client, input string, args ...string) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd, err := NewRootCmdWithDependencies(Dependencies{
		Store:      store,
		HTTPClient: client,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if err != nil {
		t.Fatalf("create root command: %v", err)
	}

	cmd.SetArgs(args)
	if input != "" {
		cmd.SetIn(strings.NewReader(input))
	}

	err = cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestAuthLoginWithTokenArgument(t *testing.T) {
	const token = "token-arg"
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0.1/me" || r.Method != http.MethodGet {
			t.Fatalf("unexpected validation request: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	stdout, _, err := executeCLI(t, store, server.Client(), "", "auth", "login", "--hostname", server.URL, "--with-token", token)
	if err != nil {
		t.Fatalf("auth login failed: %v", err)
	}
	if gotAuth != token {
		t.Fatalf("Authorization header = %q; want %q", gotAuth, token)
	}
	if !strings.Contains(stdout, "Logged in to") {
		t.Fatalf("unexpected stdout: %q", stdout)
	}

	resolved, err := store.ResolveToken(server.URL)
	if err != nil {
		t.Fatalf("resolve saved token: %v", err)
	}
	if resolved != token {
		t.Fatalf("saved token = %q; want %q", resolved, token)
	}
}

func TestAuthLoginWithTokenFromStdin(t *testing.T) {
	const token = "token-stdin"
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, _, err := executeCLI(t, store, server.Client(), token+"\n", "auth", "login", "--hostname", server.URL, "--with-token")
	if err != nil {
		t.Fatalf("auth login with stdin failed: %v", err)
	}
	if gotAuth != token {
		t.Fatalf("Authorization header = %q; want %q", gotAuth, token)
	}
}

func TestGeneratedCommandAddonsList(t *testing.T) {
	const token = "addons-token"
	var gotPath, gotMethod, gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"slug":"addon-slug","name":"addon-name","id":"addon-id"}]}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	stdout, _, err := executeCLI(t, store, server.Client(), "", "addons", "list", "--hostname", server.URL)
	if err != nil {
		t.Fatalf("addons list failed: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v0.1/addons" {
		t.Fatalf("request = %s %s; want GET /v0.1/addons", gotMethod, gotPath)
	}
	if gotAuth != token {
		t.Fatalf("Authorization header = %q; want %q", gotAuth, token)
	}
	if !strings.Contains(stdout, "ID") || !strings.Contains(stdout, "SLUG") {
		t.Fatalf("expected table headers in stdout: %q", stdout)
	}
	if !strings.Contains(stdout, "addon-id") || !strings.Contains(stdout, "addon-slug") {
		t.Fatalf("expected table values in stdout: %q", stdout)
	}
}

func TestGeneratedCommandAddonsListJSONFields(t *testing.T) {
	const token = "addons-json-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"id":"addon-id","title":"addon-title","slug":"addon-slug","name":"addon-name"}]}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	stdout, _, err := executeCLI(t, store, server.Client(), "", "addons", "list", "--hostname", server.URL, "--format", "json", "--fields", "id,title")
	if err != nil {
		t.Fatalf("addons list --fields failed: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("decode stdout: %v (stdout=%q)", err, stdout)
	}
	if len(rows) != 1 {
		t.Fatalf("rows length = %d; want 1", len(rows))
	}
	row := rows[0]
	if row["id"] != "addon-id" || row["title"] != "addon-title" {
		t.Fatalf("unexpected row values: %#v", row)
	}
	if _, ok := row["slug"]; ok {
		t.Fatalf("unexpected slug field in row: %#v", row)
	}
}

func TestGeneratedCommandAddonsListTemplate(t *testing.T) {
	const token = "addons-template-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"id":"addon-id","title":"addon-title","slug":"addon-slug","name":"addon-name"}]}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	templateArg := `{{range .}}{{.id}} {{.title}}{{"\n"}}{{end}}`
	stdout, _, err := executeCLI(
		t,
		store,
		server.Client(),
		"",
		"addons",
		"list",
		"--hostname",
		server.URL,
		"--format",
		"json",
		"--fields",
		"id,title",
		"--template",
		templateArg,
	)
	if err != nil {
		t.Fatalf("addons list --template failed: %v", err)
	}
	if strings.TrimSpace(stdout) != "addon-id addon-title" {
		t.Fatalf("unexpected template stdout: %q", stdout)
	}
}

func TestGeneratedCommandAddonsListFormatJSON(t *testing.T) {
	const token = "addons-json-format-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"id":"addon-id","title":"addon-title"}]}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	stdout, _, err := executeCLI(t, store, server.Client(), "", "addons", "list", "--hostname", server.URL, "--format", "json")
	if err != nil {
		t.Fatalf("addons list --format json failed: %v", err)
	}
	if !strings.Contains(stdout, "\"data\"") {
		t.Fatalf("expected raw json output, got: %q", stdout)
	}
}

func TestGeneratedCommandAddonsListHelpIncludesAvailableFields(t *testing.T) {
	store := newTestStore(t)
	stdout, _, err := executeCLI(t, store, nil, "", "addons", "list", "--help")
	if err != nil {
		t.Fatalf("addons list --help failed: %v", err)
	}
	if !strings.Contains(stdout, "AVAILABLE FIELDS") {
		t.Fatalf("help should include AVAILABLE FIELDS, got: %q", stdout)
	}
	if !strings.Contains(stdout, "id") {
		t.Fatalf("help should include schema fields, got: %q", stdout)
	}
}

func TestGeneratedCommandAddonsListRejectsUnknownField(t *testing.T) {
	const token = "addons-json-invalid-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"slug":"addon-slug","name":"addon-name","id":"addon-id"}]}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	_, _, err := executeCLI(t, store, server.Client(), "", "addons", "list", "--hostname", server.URL, "--fields", "id,unknown")
	if err == nil {
		t.Fatal("expected error for unknown --fields value")
	}
	if !strings.Contains(err.Error(), "unsupported --fields value(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGeneratedCommandAddonsListTemplateRejectsTableFormat(t *testing.T) {
	store := newTestStore(t)
	_, _, err := executeCLI(
		t,
		store,
		nil,
		"",
		"addons",
		"list",
		"--fields",
		"id",
		"--template",
		"{{.id}}",
		"--format",
		"table",
	)
	if err == nil {
		t.Fatal("expected error for --template with --format table")
	}
	if !strings.Contains(err.Error(), "--template cannot be used with --format table") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGeneratedCommandPostDoesNotHaveFieldsFlag(t *testing.T) {
	store := newTestStore(t)
	_, _, err := executeCLI(t, store, nil, "", "builds", "trigger", "--fields", "slug")
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --fields") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPICommandPatchRequest(t *testing.T) {
	const token = "api-token"
	var gotMethod, gotPath, gotAuth, gotHeader string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotHeader = r.Header.Get("X-Test")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	_, _, err := executeCLI(t, store, server.Client(), "", "api", "-X", "PATCH", "/apps/app/builds", "--hostname", server.URL, "-f", "name=value", "-H", "X-Test: yes")
	if err != nil {
		t.Fatalf("api patch failed: %v", err)
	}

	if gotMethod != http.MethodPatch || gotPath != "/v0.1/apps/app/builds" {
		t.Fatalf("request = %s %s; want PATCH /v0.1/apps/app/builds", gotMethod, gotPath)
	}
	if gotAuth != token {
		t.Fatalf("Authorization header = %q; want %q", gotAuth, token)
	}
	if gotHeader != "yes" {
		t.Fatalf("X-Test header = %q; want yes", gotHeader)
	}
	if gotBody["name"] != "value" {
		t.Fatalf("request body = %#v; expected name=value", gotBody)
	}
}

func TestAPICommandPaginateSlurp(t *testing.T) {
	const token = "paginate-token"
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		next := r.URL.Query().Get("next")
		w.WriteHeader(http.StatusOK)
		if next == "" {
			_, _ = io.WriteString(w, `{"data":[1],"paging":{"next":"token2"}}`)
			return
		}
		if next == "token2" {
			_, _ = io.WriteString(w, `{"data":[2],"paging":{}}`)
			return
		}
		t.Fatalf("unexpected next query: %q", next)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	stdout, _, err := executeCLI(t, store, server.Client(), "", "api", "/apps/app/builds", "--hostname", server.URL, "--paginate", "--slurp")
	if err != nil {
		t.Fatalf("api paginate slurp failed: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d; want 2", callCount)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("decode stdout as slurp JSON: %v (stdout=%q)", err, stdout)
	}
	if len(decoded) != 2 {
		t.Fatalf("slurp output length = %d; want 2", len(decoded))
	}
}

func TestAPICommandNon2xxReturnsErrorAndBody(t *testing.T) {
	const token = "error-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"bad request"}`)
	}))
	defer server.Close()

	store := newTestStore(t)
	if err := store.SaveToken(server.URL, token, true); err != nil {
		t.Fatalf("save token: %v", err)
	}

	stdout, _, err := executeCLI(t, store, server.Client(), "", "api", "/apps/app/builds", "--hostname", server.URL)
	if err == nil {
		t.Fatal("expected command error for non-2xx response")
	}
	if !strings.Contains(stdout, "bad request") {
		t.Fatalf("expected response body in stdout, got %q", stdout)
	}
}
