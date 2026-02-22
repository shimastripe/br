package generated

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/shimastripe/br/internal/hostutil"
	"github.com/shimastripe/br/internal/httpclient"
	"github.com/spf13/cobra"
)

func Commands(runner *httpclient.Runner) []*cobra.Command {
	commands := make([]*cobra.Command, 0, len(Tags))

	for _, tag := range Tags {
		tag := tag
		tagCmd := &cobra.Command{
			Use:   tag.Name,
			Short: fmt.Sprintf("Bitrise API commands for %s", tag.Name),
		}

		for _, op := range tag.Operations {
			tagCmd.AddCommand(newOperationCommand(runner, op))
		}
		commands = append(commands, tagCmd)
	}

	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name() < commands[j].Name()
	})
	return commands
}

func newOperationCommand(runner *httpclient.Runner, op OperationSpec) *cobra.Command {
	var host string
	var headers []string
	var rawFields []string
	var typedFields []string
	var inputFile string
	var jsonFields string
	var outputTemplate string
	var include bool
	var silent bool
	var verbose bool
	var paginate bool
	var slurp bool
	var jq string

	short := strings.TrimSpace(op.Summary)
	if short == "" {
		short = fmt.Sprintf("%s %s", strings.ToUpper(op.Method), op.Path)
	}

	command := &cobra.Command{
		Use:     op.Name,
		Short:   short,
		Aliases: []string{op.OperationID},
		Long:    buildOperationLong(op),
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint := op.Path
			query := url.Values{}

			for _, param := range op.Params {
				if param.In != "path" && param.In != "query" {
					continue
				}
				value, err := cmd.Flags().GetString(param.Name)
				if err != nil {
					return err
				}
				if param.Required && strings.TrimSpace(value) == "" {
					return fmt.Errorf("--%s is required", param.Name)
				}

				if param.In == "path" {
					endpoint = strings.ReplaceAll(endpoint, "{"+param.Name+"}", url.PathEscape(value))
					continue
				}

				if cmd.Flags().Changed(param.Name) {
					query.Set(param.Name, value)
				}
			}

			if len(query) > 0 {
				separator := "?"
				if strings.Contains(endpoint, "?") {
					separator = "&"
				}
				endpoint = endpoint + separator + query.Encode()
			}

			if op.BodyRequired && strings.TrimSpace(inputFile) == "" && len(rawFields) == 0 && len(typedFields) == 0 {
				return fmt.Errorf("this command requires body data: use --input, --raw-field or --field")
			}

			if slurp && !paginate {
				return fmt.Errorf("--slurp requires --paginate")
			}

			selectedFields, err := parseJSONFieldsFlag(jsonFields)
			if err != nil {
				return err
			}
			if err := validateJSONFields(selectedFields, op.JSONFields); err != nil {
				return err
			}
			if strings.TrimSpace(outputTemplate) != "" && len(selectedFields) == 0 {
				return fmt.Errorf("--template requires --json")
			}

			return runner.Execute(cmd.Context(), httpclient.RequestOptions{
				Host:           host,
				Endpoint:       endpoint,
				Method:         strings.ToUpper(op.Method),
				MethodExplicit: true,
				Headers:        headers,
				RawFields:      rawFields,
				TypedFields:    typedFields,
				InputFile:      inputFile,
				JSONFields:     selectedFields,
				Template:       outputTemplate,
				Include:        include,
				Silent:         silent,
				Verbose:        verbose,
				Paginate:       paginate,
				Slurp:          slurp,
				JQ:             jq,
			})
		},
	}

	if op.OperationID == op.Name {
		command.Aliases = nil
	}

	for _, param := range op.Params {
		if param.In != "path" && param.In != "query" {
			continue
		}
		desc := strings.TrimSpace(strings.ReplaceAll(param.Description, "\n", " "))
		if desc == "" {
			desc = fmt.Sprintf("%s parameter", param.In)
		}
		command.Flags().String(param.Name, "", desc)
		if param.Required {
			_ = command.MarkFlagRequired(param.Name)
		}
	}

	command.Flags().StringVar(&host, "hostname", hostutil.DefaultHost, "The Bitrise API hostname for the request")
	command.Flags().StringArrayVarP(&headers, "header", "H", nil, "Add a HTTP request header in key:value format")
	command.Flags().StringArrayVarP(&rawFields, "raw-field", "f", nil, "Add a string parameter in key=value format")
	command.Flags().StringArrayVarP(&typedFields, "field", "F", nil, "Add a typed parameter in key=value format (use @<path> or @-)")
	command.Flags().StringVar(&inputFile, "input", "", "The file to use as body for the request (use - for stdin)")
	if op.SupportsJSON {
		command.Flags().StringVar(&jsonFields, "json", "", "Output JSON with the specified fields (comma-separated)")
		command.Flags().StringVarP(&outputTemplate, "template", "t", "", "Format JSON output using a Go template")
	}
	command.Flags().BoolVarP(&include, "include", "i", false, "Include HTTP response status line and headers in output")
	command.Flags().BoolVar(&silent, "silent", false, "Do not print the response body")
	command.Flags().BoolVar(&verbose, "verbose", false, "Include full HTTP request and response in output")
	command.Flags().BoolVar(&paginate, "paginate", false, "Make additional requests to fetch all pages")
	command.Flags().BoolVar(&slurp, "slurp", false, "Use with --paginate to return an array of all pages")
	command.Flags().StringVarP(&jq, "jq", "q", "", "Query to select values from the response using jq syntax")

	return command
}

func buildOperationLong(op OperationSpec) string {
	parts := []string{}
	summary := strings.TrimSpace(op.Summary)
	if summary != "" {
		parts = append(parts, summary)
	}

	description := strings.TrimSpace(op.Description)
	if description != "" {
		parts = append(parts, description)
	}

	parts = append(parts,
		fmt.Sprintf("Operation ID: %s", op.OperationID),
		fmt.Sprintf("Endpoint: %s %s", strings.ToUpper(op.Method), op.Path),
	)

	if op.SupportsJSON {
		if len(op.JSONFields) > 0 {
			parts = append(parts, "JSON FIELDS\n  "+strings.Join(op.JSONFields, ", "))
		} else {
			parts = append(parts, "JSON FIELDS\n  (schema-defined fields are unavailable for this endpoint)")
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func parseJSONFieldsFlag(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	fields := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		fields = append(fields, field)
	}

	if len(fields) == 0 {
		return nil, fmt.Errorf("invalid --json value: expected comma-separated field names")
	}
	return fields, nil
}

func validateJSONFields(selected []string, allowed []string) error {
	if len(selected) == 0 || len(allowed) == 0 {
		return nil
	}

	allowedSet := map[string]struct{}{}
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}

	invalid := make([]string, 0)
	for _, field := range selected {
		if _, ok := allowedSet[field]; !ok {
			invalid = append(invalid, field)
		}
	}
	if len(invalid) == 0 {
		return nil
	}

	return fmt.Errorf(
		"unsupported --json field(s): %s (available: %s)",
		strings.Join(invalid, ", "),
		strings.Join(allowed, ", "),
	)
}
