package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"id", "key_id", "primary_key_id", "emails", "public_key", "created_at", "expires_at", "verified", "can_sign"}

// ListOptions holds all inputs for the gpg-key list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	JSON       cmdutil.JSONFlags

	// Now is the clock the ADDED column measures against.
	Now func() time.Time
}

// Email is one email of a GPG key, as Gitea returns it.
type Email struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

// Key is one GPG key of the account, as GET /user/gpg_keys returns it.
type Key struct {
	ID           int64   `json:"id"`
	KeyID        string  `json:"key_id"`
	PrimaryKeyID string  `json:"primary_key_id"`
	Emails       []Email `json:"emails"`
	PublicKey    string  `json:"public_key"`
	CreatedAt    string  `json:"created_at"`
	ExpiresAt    string  `json:"expires_at"`
	Verified     bool    `json:"verified"`
	CanSign      bool    `json:"can_sign"`
}

// emails joins the key's emails with ", ", as gh does.
func (k Key) emails() string {
	list := make([]string, len(k.Emails))
	for i, e := range k.Emails {
		list[i] = e.Email
	}
	return strings.Join(list, ", ")
}

// NewCmdList creates the `copia-cli gpg-key list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List GPG keys in your Copia account",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Example: `  $ copia-cli gpg-key list
  $ copia-cli gpg-key list --json key_id,verified`,
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

// FetchKeys reads every page of the GPG keys of the authenticated user.
func FetchKeys(client *http.Client, host, token string) ([]Key, error) {
	keys, err := cmdutil.FetchPages(client, fmt.Sprintf("https://%s/api/v1/user/gpg_keys", host), token, 0, decodeKeys)
	if err != nil {
		var httpErr *cmdutil.HTTPError
		if errors.As(err, &httpErr) {
			if httpErr.Message != "" {
				return nil, fmt.Errorf("failed to list GPG keys (HTTP %d): %s", httpErr.StatusCode, httpErr.Message)
			}
			return nil, fmt.Errorf("failed to list GPG keys (HTTP %d)", httpErr.StatusCode)
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
		if keys == nil {
			keys = []Key{}
		}
		return opts.JSON.Print(opts.IO.Out, keys)
	}

	if len(keys) == 0 {
		_, _ = fmt.Fprintln(opts.IO.ErrOut, "no GPG keys present in the Copia account")
		return nil
	}

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if !opts.IO.IsStdoutTTY() {
		for _, k := range keys {
			_, _ = fmt.Fprintf(opts.IO.Out, "%s\t%s\t%s\t%s\t%s\n", k.emails(), k.KeyID, k.PublicKey, k.CreatedAt, k.ExpiresAt)
		}
		return nil
	}

	now := opts.Now()
	rows := make([][5]string, len(keys))
	widths := [5]int{len("EMAIL"), len("KEY ID"), len("PUBLIC KEY"), len("ADDED"), len("EXPIRES")}
	for i, k := range keys {
		rows[i] = [5]string{k.emails(), k.KeyID, k.PublicKey, timeAgo(now, k.CreatedAt), expires(k.ExpiresAt)}
		for c, v := range rows[i] {
			widths[c] = max(widths[c], len(v))
		}
	}
	// The public key takes what the other columns and the padding leave of
	// the terminal width, cut in the middle as gh does.
	keyWidth := opts.IO.TerminalWidth() - widths[0] - widths[1] - widths[3] - widths[4] - 4*2
	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "EMAIL\tKEY ID\tPUBLIC KEY\tADDED\tEXPIRES")
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r[0], r[1], truncateMiddle(max(keyWidth, len("PUBLIC KEY")), r[2]), r[3], r[4])
	}
	return w.Flush()
}

// expires is the EXPIRES column on a terminal: Never for a key without an
// expiry, which Gitea gives as the zero time or the Unix epoch, otherwise
// the date. A time that does not parse is returned unchanged.
func expires(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	if t.Unix() <= 0 {
		return "Never"
	}
	return t.Format("2006-01-02")
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
// gpg-key list does; a public key in base64 is ASCII.
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
