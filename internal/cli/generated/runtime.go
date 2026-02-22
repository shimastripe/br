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
		Long: strings.TrimSpace(strings.Join([]string{
			strings.TrimSpace(op.Summary),
			strings.TrimSpace(op.Description),
			fmt.Sprintf("Operation ID: %s", op.OperationID),
			fmt.Sprintf("Endpoint: %s %s", strings.ToUpper(op.Method), op.Path),
		}, "\n\n")),
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

			return runner.Execute(cmd.Context(), httpclient.RequestOptions{
				Host:           host,
				Endpoint:       endpoint,
				Method:         strings.ToUpper(op.Method),
				MethodExplicit: true,
				Headers:        headers,
				RawFields:      rawFields,
				TypedFields:    typedFields,
				InputFile:      inputFile,
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
	command.Flags().BoolVarP(&include, "include", "i", false, "Include HTTP response status line and headers in output")
	command.Flags().BoolVar(&silent, "silent", false, "Do not print the response body")
	command.Flags().BoolVar(&verbose, "verbose", false, "Include full HTTP request and response in output")
	command.Flags().BoolVar(&paginate, "paginate", false, "Make additional requests to fetch all pages")
	command.Flags().BoolVar(&slurp, "slurp", false, "Use with --paginate to return an array of all pages")
	command.Flags().StringVarP(&jq, "jq", "q", "", "Query to select values from the response using jq syntax")

	return command
}
