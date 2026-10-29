package fork

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type ForkOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Org        string
}

type forkRequest struct {
	Organization string `json:"organization,omitempty"`
}

type forkResponse struct {
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
	CloneURL string `json:"clone_url"`
}

func NewCmdFork(f *cmdutil.Factory) *cobra.Command {
	opts := &ForkOptions{}

	cmd := &cobra.Command{
		Use:   "fork <owner/repo>",
		Short: "Fork a repository",
		Long:  "Create a fork of a Copia repository. By default the fork is created under the authenticated user. Use --org to fork into an organization.",
		Example: `  # Fork a repository
  $ copia-cli repo fork upstream-org/project

  # Fork into an organization
  $ copia-cli repo fork upstream-org/project --org my-org`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			owner, repo, err := cmdutil.SplitOwnerRepo(args[0])
			if err != nil {
				return err
			}
			opts.Owner, opts.Repo = owner, repo
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ForkRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "Fork to organization")

	return cmd
}

func ForkRun(opts *ForkOptions) error {
	payload := forkRequest{Organization: opts.Org}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/forks", opts.Host, opts.Owner, opts.Repo)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fork repository (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result forkResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Forked to %s\n%s\n", result.FullName, result.HTMLURL)
	return nil
}

