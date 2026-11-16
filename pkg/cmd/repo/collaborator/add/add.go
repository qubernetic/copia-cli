package add

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// permissions are the values Gitea takes; it answers any other with 500.
var permissions = []string{"read", "write", "admin"}

// loginPattern is the shape of a Gitea login. A login is a path segment of
// the request, so one that is not a login is refused before it is sent.
var loginPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// AddOptions holds all inputs for the repo collaborator add command.
type AddOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Login      string
	Permission string
}

type addRequest struct {
	Permission string `json:"permission"`
}

// NewCmdAdd creates the `copia-cli repo collaborator add` command.
func NewCmdAdd(f *cmdutil.Factory) *cobra.Command {
	opts := &AddOptions{}

	cmd := &cobra.Command{
		Use:   "add <login>",
		Short: "Add a collaborator to a repository",
		Long: `Add a user as a collaborator of a Copia repository with read, write or admin
permission (default write). For a user who is a collaborator already, the
permission is changed to the given one.`,
		Args: cobra.ExactArgs(1),
		Example: `  $ copia-cli repo collaborator add monalisa
  $ copia-cli repo collaborator add monalisa --permission read
  $ copia-cli repo collaborator add monalisa -p admin -R my-org/my-repo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Login = args[0]
			if err := validate(opts); err != nil {
				return err
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			owner, repo, err := f.ResolveRepo()
			if err != nil {
				return err
			}
			opts.Owner = owner
			opts.Repo = repo
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return AddRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Permission, "permission", "p", "write", "Permission of the collaborator: {read|write|admin}")

	return cmd
}

// validate refuses a login or permission Gitea would not take.
func validate(opts *AddOptions) error {
	if !loginPattern.MatchString(opts.Login) {
		return fmt.Errorf("invalid login %q", opts.Login)
	}
	if !slices.Contains(permissions, opts.Permission) {
		return fmt.Errorf("invalid permission %q: use %s or %s", opts.Permission, strings.Join(permissions[:len(permissions)-1], ", "), permissions[len(permissions)-1])
	}
	return nil
}

func AddRun(opts *AddOptions) error {
	if err := validate(opts); err != nil {
		return err
	}

	body, err := json.Marshal(addRequest{Permission: opts.Permission})
	if err != nil {
		return err
	}
	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/collaborators/%s", opts.Host, url.PathEscape(opts.Owner), url.PathEscape(opts.Repo), url.PathEscape(opts.Login))
	req, err := http.NewRequest("PUT", u, bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusNoContent {
		message := cmdutil.ReadAPIMessage(resp.Body)
		switch {
		// Gitea answers an unknown user with 422 and "user ... does not
		// exist [...]".
		case resp.StatusCode == http.StatusUnprocessableEntity && (message == "" || strings.Contains(message, "does not exist")):
			return fmt.Errorf("user %s not found", opts.Login)
		case message != "":
			return fmt.Errorf("failed to add collaborator (HTTP %d): %s", resp.StatusCode, message)
		}
		return fmt.Errorf("failed to add collaborator (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Added %s to %s/%s with %s permission\n", opts.Login, opts.Owner, opts.Repo, opts.Permission)
	return nil
}
