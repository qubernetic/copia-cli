package view

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Name       string
	JSON       cmdutil.JSONFlags
}

type orgDetail struct {
	Username    string `json:"username"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
}

func NewCmdView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{}

	cmd := &cobra.Command{
		Use:     "view <org>",
		Short:   "View an organization",
		Long:    "Display information about a Copia organization, including its description and member visibility settings.",
		Example: "  $ copia-cli org view my-org",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.IO = f.IOStreams
			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ViewRun(opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.JSON, []string{"username", "full_name", "description", "website"})
	return cmd
}

func ViewRun(opts *ViewOptions) error {
	url := fmt.Sprintf("https://%s/api/v1/orgs/%s", opts.Host, opts.Name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("organization %s not found", opts.Name)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var org orgDetail
	if err := json.Unmarshal(body, &org); err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, org)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "%s (%s)\n", org.Username, org.FullName)
	if org.Description != "" {
		_, _ = fmt.Fprintf(opts.IO.Out, "%s\n", org.Description)
	}
	if org.Website != "" {
		_, _ = fmt.Fprintf(opts.IO.Out, "Website: %s\n", org.Website)
	}
	return nil
}
