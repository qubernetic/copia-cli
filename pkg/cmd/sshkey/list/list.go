package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"id", "title", "key", "fingerprint", "created_at", "last_used_at"}

// keyType is the TYPE column of every key: Gitea has no SSH signing keys.
const keyType = "authentication"

// ListOptions holds all inputs for the ssh-key list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	JSON       cmdutil.JSONFlags

	// Now is the clock the ADDED column measures against.
	Now func() time.Time
}

// Key is one SSH key of the account, as GET /user/keys returns it.
type Key struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
	LastUsedAt  string `json:"last_used_at"`
}

// NewCmdList creates the `copia-cli ssh-key list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List SSH keys in your Copia account",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Example: `  $ copia-cli ssh-key list
  $ copia-cli ssh-key list --json id,title,fingerprint`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			opts.Now = time.Now
			return ListRun(opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodeKeys(body []byte) ([]Key, error) {
	var keys []Key
	err := json.Unmarshal(body, &keys)
	return keys, err
}

// FetchKeys reads every page of the SSH keys of the authenticated user.
func FetchKeys(client *http.Client, host, token string) ([]Key, error) {
	keys, err := cmdutil.FetchPages(client, fmt.Sprintf("https://%s/api/v1/user/keys", host), token, 0, decodeKeys)
	if err != nil {
		var httpErr *cmdutil.HTTPError
		if errors.As(err, &httpErr) {
			if httpErr.Message != "" {
				return nil, fmt.Errorf("failed to list SSH keys (HTTP %d): %s", httpErr.StatusCode, httpErr.Message)
			}
			return nil, fmt.Errorf("failed to list SSH keys (HTTP %d)", httpErr.StatusCode)
		}
		return nil, err
	}
	return keys, nil
}

func ListRun(opts *ListOptions) error {
	keys, err := FetchKeys(opts.HTTPClient, opts.Host, opts.Token)
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, keys)
	}

	if len(keys) == 0 {
		_, _ = fmt.Fprintln(opts.IO.ErrOut, "no SSH keys present in the Copia account")
		return nil
	}

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if !opts.IO.IsStdoutTTY() {
		for _, k := range keys {
			_, _ = fmt.Fprintf(opts.IO.Out, "%s\t%s\t%s\t%d\t%s\n", k.Title, k.Key, k.CreatedAt, k.ID, keyType)
		}
		return nil
	}

	now := opts.Now()
	rows := make([][5]string, len(keys))
	widths := [5]int{len("TITLE"), len("ID"), len("KEY"), len("TYPE"), len("ADDED")}
	for i, k := range keys {
		rows[i] = [5]string{k.Title, strconv.FormatInt(k.ID, 10), k.Key, keyType, timeAgo(now, k.CreatedAt)}
		for c, v := range rows[i] {
			widths[c] = max(widths[c], len(v))
		}
	}
	// The key takes what the other columns and the padding leave of the
	// terminal width, cut in the middle as gh does.
	keyWidth := opts.IO.TerminalWidth() - widths[0] - widths[1] - widths[3] - widths[4] - 4*2
	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TITLE\tID\tKEY\tTYPE\tADDED")
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r[0], r[1], truncateMiddle(max(keyWidth, len("KEY")), r[2]), r[3], r[4])
	}
	return w.Flush()
}

// timeAgo describes an RFC 3339 time in words, or returns it unchanged when
// it does not parse.
func timeAgo(now time.Time, s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return cmdutil.TimeAgo(now, t)
}

// truncateMiddle cuts t to maxWidth bytes with "..." in the middle, as gh's
// ssh-key list does; an SSH public key is ASCII.
func truncateMiddle(maxWidth int, t string) string {
	if len(t) <= maxWidth {
		return t
	}

	ellipsis := "..."
	if maxWidth < len(ellipsis)+2 {
		return t[0:maxWidth]
	}

	halfWidth := (maxWidth - len(ellipsis)) / 2
	remainder := (maxWidth - len(ellipsis)) % 2
	return t[0:halfWidth+remainder] + ellipsis + t[len(t)-halfWidth:]
}
