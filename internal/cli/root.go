package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/shimastripe/br/internal/auth"
	"github.com/shimastripe/br/internal/cli/apicmd"
	"github.com/shimastripe/br/internal/cli/authcmd"
	"github.com/shimastripe/br/internal/cli/generated"
	"github.com/shimastripe/br/internal/httpclient"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Store      *auth.Store
	Runner     *httpclient.Runner
	HTTPClient *http.Client
	Stdout     io.Writer
	Stderr     io.Writer
}

func NewRootCmd() *cobra.Command {
	cmd, err := NewRootCmdWithDependencies(Dependencies{})
	if err == nil {
		return cmd
	}

	fallback := &cobra.Command{
		Use:          "br",
		Short:        "Bitrise CLI",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return err
		},
	}
	return fallback
}

func NewRootCmdWithDependencies(deps Dependencies) (*cobra.Command, error) {
	if deps.Stdout == nil {
		deps.Stdout = os.Stdout
	}
	if deps.Stderr == nil {
		deps.Stderr = os.Stderr
	}

	store := deps.Store
	if store == nil {
		var err error
		store, err = auth.NewStore()
		if err != nil {
			return nil, fmt.Errorf("initialize auth store: %w", err)
		}
	}

	runner := deps.Runner
	if runner == nil {
		runner = httpclient.NewRunner(store.ResolveToken, deps.Stdout, deps.Stderr, deps.HTTPClient)
	}

	root := &cobra.Command{
		Use:          "br",
		Short:        "Bitrise CLI",
		SilenceUsage: true,
	}

	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)

	root.AddCommand(authcmd.NewCommand(authcmd.Dependencies{
		Store:      store,
		HTTPClient: runner.HTTPClient,
		Stdout:     deps.Stdout,
	}))
	root.AddCommand(apicmd.NewCommand(runner))
	for _, command := range generated.Commands(runner) {
		root.AddCommand(command)
	}

	return root, nil
}
