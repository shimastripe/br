package apicmd

import (
	"fmt"

	"github.com/shimastripe/br/internal/hostutil"
	"github.com/shimastripe/br/internal/httpclient"
	"github.com/spf13/cobra"
)

func NewCommand(runner *httpclient.Runner) *cobra.Command {
	var host string
	var method string
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

	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make a Bitrise API request",
		Long:  "Run authenticated requests against Bitrise REST API.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := httpclient.RequestOptions{
				Host:           host,
				Endpoint:       args[0],
				Method:         method,
				MethodExplicit: cmd.Flags().Changed("method"),
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
			}
			if slurp && !paginate {
				return fmt.Errorf("--slurp requires --paginate")
			}
			return runner.Execute(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&host, "hostname", hostutil.DefaultHost, "The Bitrise API hostname for the request")
	cmd.Flags().StringVarP(&method, "method", "X", "GET", "The HTTP method for the request")
	cmd.Flags().StringArrayVarP(&headers, "header", "H", nil, "Add a HTTP request header in key:value format")
	cmd.Flags().StringArrayVarP(&rawFields, "raw-field", "f", nil, "Add a string parameter in key=value format")
	cmd.Flags().StringArrayVarP(&typedFields, "field", "F", nil, "Add a typed parameter in key=value format (use @<path> or @-)")
	cmd.Flags().StringVar(&inputFile, "input", "", "The file to use as body for the request (use - for stdin)")
	cmd.Flags().BoolVarP(&include, "include", "i", false, "Include HTTP response status line and headers in output")
	cmd.Flags().BoolVar(&silent, "silent", false, "Do not print the response body")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Include full HTTP request and response in output")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "Make additional requests to fetch all pages")
	cmd.Flags().BoolVar(&slurp, "slurp", false, "Use with --paginate to return an array of all pages")
	cmd.Flags().StringVarP(&jq, "jq", "q", "", "Query to select values from the response using jq syntax")

	return cmd
}
