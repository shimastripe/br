package authcmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shimastripe/br/internal/auth"
	"github.com/shimastripe/br/internal/hostutil"
	"github.com/spf13/cobra"
)

type Validator func(ctx context.Context, host string, token string) error

type Dependencies struct {
	Store      *auth.Store
	HTTPClient *http.Client
	Stdout     io.Writer
	Validator  Validator
}

func NewCommand(deps Dependencies) *cobra.Command {
	if deps.Stdout == nil {
		deps.Stdout = os.Stdout
	}
	if deps.HTTPClient == nil {
		deps.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if deps.Validator == nil {
		deps.Validator = makeTokenValidator(deps.HTTPClient)
	}

	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with Bitrise",
	}
	authCmd.AddCommand(newLoginCommand(deps), newStatusCommand(deps), newLogoutCommand(deps))
	return authCmd
}

func newLoginCommand(deps Dependencies) *cobra.Command {
	var withToken bool
	var hostname string
	var insecureStorage bool

	cmd := &cobra.Command{
		Use:   "login [token]",
		Short: "Log in to Bitrise",
		Long:  "Authenticate by token and store credentials in keychain by default.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !withToken {
				return fmt.Errorf("--with-token is required")
			}

			tokenInput := ""
			if len(args) > 0 {
				tokenInput = args[0]
			} else {
				bytes, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read token from stdin: %w", err)
				}
				tokenInput = string(bytes)
			}
			tokenInput = strings.TrimSpace(tokenInput)
			if tokenInput == "" {
				return fmt.Errorf("token is empty")
			}

			if err := deps.Validator(cmd.Context(), hostname, tokenInput); err != nil {
				return err
			}

			if err := deps.Store.SaveToken(hostname, tokenInput, insecureStorage); err != nil {
				return err
			}

			_, normalizedHost, err := hostutil.Normalize(hostname)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(deps.Stdout, "Logged in to %s\n", normalizedHost)
			return nil
		},
	}

	cmd.Flags().BoolVar(&withToken, "with-token", false, "Read token from positional argument or stdin")
	cmd.Flags().StringVar(&hostname, "hostname", hostutil.DefaultHost, "The Bitrise API host to authenticate with")
	cmd.Flags().BoolVar(&insecureStorage, "insecure-storage", false, "Save token in plain text config file instead of keychain")

	return cmd
}

func newStatusCommand(deps Dependencies) *cobra.Command {
	var hostname string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, host, err := hostutil.Normalize(hostname)
			if err != nil {
				return err
			}

			_, source, err := deps.Store.ResolveTokenWithSource(host)
			if err != nil {
				if err == auth.ErrNoToken {
					_, _ = fmt.Fprintf(deps.Stdout, "Not logged in to %s\n", host)
					return err
				}
				return err
			}

			_, _ = fmt.Fprintf(deps.Stdout, "Logged in to %s (source: %s)\n", host, source)
			return nil
		},
	}
	cmd.Flags().StringVar(&hostname, "hostname", hostutil.DefaultHost, "The Bitrise API host")
	return cmd
}

func newLogoutCommand(deps Dependencies) *cobra.Command {
	var hostname string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out from Bitrise",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.Store.DeleteToken(hostname); err != nil {
				return err
			}
			_, host, err := hostutil.Normalize(hostname)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(deps.Stdout, "Logged out from %s\n", host)
			return nil
		},
	}
	cmd.Flags().StringVar(&hostname, "hostname", hostutil.DefaultHost, "The Bitrise API host")
	return cmd
}

func makeTokenValidator(client *http.Client) Validator {
	return func(ctx context.Context, host string, token string) error {
		scheme, normalizedHost, err := hostutil.Normalize(host)
		if err != nil {
			return err
		}
		url := fmt.Sprintf("%s://%s/v0.1/me", scheme, normalizedHost)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build validation request: %w", err)
		}
		req.Header.Set("Authorization", strings.TrimSpace(token))

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("token validation request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 {
				return fmt.Errorf("token validation failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
			}
			return fmt.Errorf("token validation failed: %s", resp.Status)
		}

		return nil
	}
}
