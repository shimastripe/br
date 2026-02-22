package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/itchyny/gojq"
	"github.com/shimastripe/br/internal/hostutil"
)

const defaultBasePath = "/v0.1"

type TokenResolver func(host string) (string, error)

type Runner struct {
	HTTPClient    *http.Client
	TokenResolver TokenResolver
	Stdout        io.Writer
	Stderr        io.Writer
}

type RequestOptions struct {
	Host           string
	Endpoint       string
	Method         string
	MethodExplicit bool
	Headers        []string
	RawFields      []string
	TypedFields    []string
	InputFile      string
	JSONFields     []string
	Template       string
	Include        bool
	Silent         bool
	Verbose        bool
	Paginate       bool
	Slurp          bool
	JQ             string
}

type HTTPError struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (e *HTTPError) Error() string {
	return "request failed: " + e.Status
}

func NewRunner(resolver TokenResolver, stdout io.Writer, stderr io.Writer, client *http.Client) *Runner {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Runner{
		HTTPClient:    client,
		TokenResolver: resolver,
		Stdout:        stdout,
		Stderr:        stderr,
	}
}

func (r *Runner) Execute(ctx context.Context, opts RequestOptions) error {
	scheme, host, err := hostutil.Normalize(opts.Host)
	if err != nil {
		return err
	}

	method, query, body, err := prepareMethodQueryBody(opts, os.Stdin)
	if err != nil {
		return err
	}

	resolvedURL, err := resolveEndpointURL(scheme, host, opts.Endpoint, query)
	if err != nil {
		return err
	}

	headers, err := parseHeaders(opts.Headers)
	if err != nil {
		return err
	}

	if !hasHeader(headers, "Authorization") && r.TokenResolver != nil {
		token, tokenErr := r.TokenResolver(host)
		if tokenErr != nil {
			return tokenErr
		}
		headers.Set("Authorization", strings.TrimSpace(token))
	}

	pages := make([][]byte, 0, 1)
	requestURL := resolvedURL
	for i := 0; ; i++ {
		statusCode, pageBody, reqErr := r.doRequest(ctx, method, requestURL, headers, body, opts)
		if reqErr != nil {
			if httpErr := (&HTTPError{}); errors.As(reqErr, &httpErr) {
				if !opts.Silent && len(httpErr.Body) > 0 {
					writeBytes(r.Stdout, httpErr.Body)
				}
			}
			return reqErr
		}

		_ = statusCode
		pages = append(pages, pageBody)

		if !opts.Paginate {
			break
		}
		nextAnchor, nextErr := extractNextAnchor(pageBody)
		if nextErr != nil {
			return nextErr
		}
		if nextAnchor == "" {
			break
		}

		requestURL, err = nextURL(resolvedURL, nextAnchor)
		if err != nil {
			return err
		}
	}

	if opts.Silent {
		return nil
	}

	if opts.JQ != "" && opts.Template == "" && len(opts.JSONFields) == 0 {
		if opts.Paginate && !opts.Slurp && len(pages) > 1 {
			for _, page := range pages {
				filtered, filterErr := applyJQ(opts.JQ, page)
				if filterErr != nil {
					return filterErr
				}
				writeBytes(r.Stdout, filtered)
			}
			return nil
		}
	}

	result, err := combinePages(pages, opts.Slurp)
	if err != nil {
		return err
	}

	if len(opts.JSONFields) > 0 {
		result, err = selectJSONFields(result, opts.JSONFields)
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(opts.Template) != "" {
		rendered, renderErr := applyTemplate(opts.Template, result)
		if renderErr != nil {
			return renderErr
		}
		writeBytes(r.Stdout, rendered)
		return nil
	}

	if opts.JQ != "" {
		filtered, filterErr := applyJQ(opts.JQ, result)
		if filterErr != nil {
			return filterErr
		}
		writeBytes(r.Stdout, filtered)
		return nil
	}

	writeBytes(r.Stdout, result)
	return nil
}

func (r *Runner) doRequest(ctx context.Context, method string, endpoint string, headers http.Header, body []byte, opts RequestOptions) (int, []byte, error) {
	reader := bytes.NewReader(body)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}

	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	if len(body) > 0 && !hasHeader(req.Header, "Content-Type") {
		req.Header.Set("Content-Type", "application/json")
	}

	if opts.Verbose {
		printRequest(r.Stderr, req, body)
	}

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response body: %w", err)
	}

	if opts.Verbose {
		printResponse(r.Stderr, resp, payload)
	}

	if opts.Include {
		printResponseHeaders(r.Stdout, resp)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, payload, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, Body: payload}
	}

	return resp.StatusCode, payload, nil
}

func prepareMethodQueryBody(opts RequestOptions, stdin io.Reader) (string, url.Values, []byte, error) {
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = http.MethodGet
	}

	typedFields := make(map[string]any, len(opts.TypedFields))
	for _, field := range opts.TypedFields {
		key, rawValue, err := splitField(field)
		if err != nil {
			return "", nil, nil, err
		}
		value, err := convertTypedField(rawValue, stdin)
		if err != nil {
			return "", nil, nil, fmt.Errorf("parse --field %q: %w", field, err)
		}
		typedFields[key] = value
	}

	rawFields := make(map[string]string, len(opts.RawFields))
	for _, field := range opts.RawFields {
		key, rawValue, err := splitField(field)
		if err != nil {
			return "", nil, nil, err
		}
		rawFields[key] = rawValue
	}

	hasAnyField := len(typedFields) > 0 || len(rawFields) > 0
	if !opts.MethodExplicit && hasAnyField {
		method = http.MethodPost
	}

	query := url.Values{}

	if strings.TrimSpace(opts.InputFile) != "" {
		body, err := readInput(opts.InputFile, stdin)
		if err != nil {
			return "", nil, nil, err
		}
		for key, value := range rawFields {
			query.Set(key, value)
		}
		for key, value := range typedFields {
			query.Set(key, valueToQueryString(value))
		}
		return method, query, body, nil
	}

	if method == http.MethodGet {
		for key, value := range rawFields {
			query.Set(key, value)
		}
		for key, value := range typedFields {
			query.Set(key, valueToQueryString(value))
		}
		return method, query, nil, nil
	}

	if !hasAnyField {
		return method, query, nil, nil
	}

	bodyMap := make(map[string]any, len(rawFields)+len(typedFields))
	for key, value := range rawFields {
		bodyMap[key] = value
	}
	for key, value := range typedFields {
		bodyMap[key] = value
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal request body: %w", err)
	}
	return method, query, body, nil
}

func resolveEndpointURL(scheme string, host string, endpoint string, query url.Values) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("endpoint is required")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint %q: %w", endpoint, err)
	}

	if parsed.Scheme == "" {
		if !strings.HasPrefix(parsed.Path, "/") {
			parsed.Path = "/" + parsed.Path
		}
		parsed.Path = ensureBasePath(parsed.Path)
		parsed.Scheme = scheme
		parsed.Host = host
	} else {
		parsed.Path = ensureBasePath(parsed.Path)
	}

	merged := parsed.Query()
	for key, values := range query {
		for _, value := range values {
			merged.Set(key, value)
		}
	}
	parsed.RawQuery = merged.Encode()

	return parsed.String(), nil
}

func ensureBasePath(path string) string {
	if path == "" {
		return defaultBasePath
	}
	if path == defaultBasePath || strings.HasPrefix(path, defaultBasePath+"/") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return defaultBasePath + path
}

func splitField(field string) (string, string, error) {
	sep := strings.Index(field, "=")
	if sep <= 0 {
		return "", "", fmt.Errorf("invalid field %q: expected key=value", field)
	}
	key := strings.TrimSpace(field[:sep])
	if key == "" {
		return "", "", fmt.Errorf("invalid field %q: empty key", field)
	}
	return key, field[sep+1:], nil
}

func convertTypedField(raw string, stdin io.Reader) (any, error) {
	if strings.HasPrefix(raw, "@") {
		content, err := readInput(strings.TrimPrefix(raw, "@"), stdin)
		if err != nil {
			return nil, err
		}
		return string(content), nil
	}

	if raw == "null" {
		return nil, nil
	}
	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}

	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil && strings.ContainsAny(raw, ".eE") {
		return f, nil
	}

	return raw, nil
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		if stdin == nil {
			stdin = os.Stdin
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input file %q: %w", path, err)
	}
	return data, nil
}

func valueToQueryString(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

func parseHeaders(raw []string) (http.Header, error) {
	headers := make(http.Header)
	for _, entry := range raw {
		idx := strings.Index(entry, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid header %q: expected key:value", entry)
		}
		key := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(entry[:idx]))
		if key == "" {
			return nil, fmt.Errorf("invalid header %q: empty key", entry)
		}
		value := strings.TrimSpace(entry[idx+1:])
		headers.Add(key, value)
	}
	return headers, nil
}

func hasHeader(headers http.Header, name string) bool {
	_, ok := headers[textproto.CanonicalMIMEHeaderKey(name)]
	return ok
}

func extractNextAnchor(body []byte) (string, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", nil
	}
	paging, ok := decoded["paging"].(map[string]any)
	if !ok {
		return "", nil
	}
	next, _ := paging["next"].(string)
	return strings.TrimSpace(next), nil
}

func nextURL(base string, next string) (string, error) {
	next = strings.TrimSpace(next)
	if next == "" {
		return base, nil
	}
	if strings.HasPrefix(next, "http://") || strings.HasPrefix(next, "https://") {
		return next, nil
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	query := parsed.Query()
	query.Set("next", next)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func combinePages(pages [][]byte, slurp bool) ([]byte, error) {
	if len(pages) == 0 {
		return []byte{}, nil
	}
	if !slurp {
		if len(pages) == 1 {
			return pages[0], nil
		}
		return bytes.Join(pages, []byte("\n")), nil
	}

	items := make([]any, 0, len(pages))
	for _, page := range pages {
		var value any
		if err := json.Unmarshal(page, &value); err != nil {
			return nil, fmt.Errorf("--slurp requires JSON responses: %w", err)
		}
		items = append(items, value)
	}
	joined, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal slurped response: %w", err)
	}
	return joined, nil
}

func applyJQ(expr string, payload []byte) ([]byte, error) {
	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("--jq requires JSON response: %w", err)
	}

	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("parse jq expression: %w", err)
	}

	iter := query.Run(data)
	var lines [][]byte
	for {
		value, ok := iter.Next()
		if !ok {
			break
		}
		if runErr, ok := value.(error); ok {
			return nil, fmt.Errorf("evaluate jq expression: %w", runErr)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode jq result: %w", err)
		}
		lines = append(lines, encoded)
	}

	if len(lines) == 0 {
		return []byte{}, nil
	}
	return append(bytes.Join(lines, []byte("\n")), '\n'), nil
}

func selectJSONFields(payload []byte, fields []string) ([]byte, error) {
	selected := normalizeJSONFields(fields)
	if len(selected) == 0 {
		return nil, fmt.Errorf("--json requires at least one field")
	}

	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("--json requires JSON response: %w", err)
	}

	filtered, err := filterJSONFields(data, selected)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, fmt.Errorf("encode --json output: %w", err)
	}
	return encoded, nil
}

func normalizeJSONFields(fields []string) []string {
	normalized := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, entry := range fields {
		for _, part := range strings.Split(entry, ",") {
			field := strings.TrimSpace(part)
			if field == "" {
				continue
			}
			if _, ok := seen[field]; ok {
				continue
			}
			seen[field] = struct{}{}
			normalized = append(normalized, field)
		}
	}
	return normalized
}

func filterJSONFields(data any, fields []string) (any, error) {
	projected := projectJSONValue(data)

	if rows, ok := asObjectRows(projected); ok {
		filteredRows := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			filtered := make(map[string]any, len(fields))
			for _, field := range fields {
				filtered[field] = lookupField(row, field)
			}
			filteredRows = append(filteredRows, filtered)
		}
		return filteredRows, nil
	}

	object, ok := projected.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("--json supports object responses and list responses")
	}

	filtered := make(map[string]any, len(fields))
	for _, field := range fields {
		filtered[field] = lookupField(object, field)
	}
	return filtered, nil
}

func lookupField(object map[string]any, path string) any {
	current := any(object)
	for _, part := range strings.Split(path, ".") {
		segment := strings.TrimSpace(part)
		if segment == "" {
			return nil
		}
		typed, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		next, ok := typed[segment]
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func applyTemplate(tmpl string, payload []byte) ([]byte, error) {
	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("--template requires JSON response: %w", err)
	}

	compiled, err := template.New("output").Funcs(template.FuncMap{
		"json": func(v any) (string, error) {
			encoded, encodeErr := json.Marshal(v)
			if encodeErr != nil {
				return "", encodeErr
			}
			return string(encoded), nil
		},
	}).Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse --template value: %w", err)
	}

	var out bytes.Buffer
	if err := compiled.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("execute --template value: %w", err)
	}
	return out.Bytes(), nil
}

func projectJSONValue(data any) any {
	object, ok := data.(map[string]any)
	if !ok {
		return data
	}

	raw, ok := object["data"]
	if !ok {
		return data
	}

	switch typed := raw.(type) {
	case map[string]any:
		return typed
	case []any:
		return typed
	default:
		return data
	}
}

func asObjectRows(data any) ([]map[string]any, bool) {
	typed, ok := data.([]any)
	if !ok {
		return nil, false
	}
	rows := make([]map[string]any, 0, len(typed))
	for _, item := range typed {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		rows = append(rows, object)
	}
	return rows, true
}

func printRequest(w io.Writer, req *http.Request, body []byte) {
	fmt.Fprintf(w, "> %s %s\n", req.Method, req.URL.String())
	keys := sortedHeaderKeys(req.Header)
	for _, key := range keys {
		for _, value := range req.Header[key] {
			fmt.Fprintf(w, "> %s: %s\n", key, value)
		}
	}
	if len(body) > 0 {
		fmt.Fprintf(w, ">\n> %s\n", string(body))
	}
}

func printResponse(w io.Writer, resp *http.Response, body []byte) {
	fmt.Fprintf(w, "< %s %s\n", resp.Proto, resp.Status)
	keys := sortedHeaderKeys(resp.Header)
	for _, key := range keys {
		for _, value := range resp.Header[key] {
			fmt.Fprintf(w, "< %s: %s\n", key, value)
		}
	}
	if len(body) > 0 {
		fmt.Fprintf(w, "<\n< %s\n", string(body))
	}
}

func printResponseHeaders(w io.Writer, resp *http.Response) {
	fmt.Fprintf(w, "%s %s\n", resp.Proto, resp.Status)
	keys := sortedHeaderKeys(resp.Header)
	for _, key := range keys {
		for _, value := range resp.Header[key] {
			fmt.Fprintf(w, "%s: %s\n", key, value)
		}
	}
	fmt.Fprintln(w)
}

func sortedHeaderKeys(headers http.Header) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeBytes(w io.Writer, data []byte) {
	if len(data) == 0 {
		return
	}
	_, _ = w.Write(data)
	if data[len(data)-1] != '\n' {
		_, _ = w.Write([]byte("\n"))
	}
}
