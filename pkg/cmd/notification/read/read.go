package read

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type ReadOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	ThreadID   int64
	All        bool
}

func NewCmdRead(f *cmdutil.Factory) *cobra.Command {
	opts := &ReadOptions{}

	cmd := &cobra.Command{
		Use:   "read [<thread-id>]",
		Short: "Mark notifications as read",
		Long:  "Mark notifications as read. Specify a thread ID to mark a single notification, or use --all to mark all notifications as read.",
		Example: `  # Mark all notifications as read
  $ copia-cli notification read --all

  # Mark a single notification as read
  $ copia-cli notification read 42`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			if len(args) > 0 {
				id, err := strconv.ParseInt(args[0], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid thread ID: %s", args[0])
				}
				opts.ThreadID = id
			}

			if opts.ThreadID == 0 && !opts.All {
				return fmt.Errorf("specify a thread ID or --all")
			}

			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ReadRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.All, "all", false, "Mark all notifications as read")

	return cmd
}

func ReadRun(opts *ReadOptions) error {
	var method, url string

	if opts.All {
		method = "PUT"
		url = fmt.Sprintf("https://%s/api/v1/notifications", opts.Host)
	} else {
		method = "PATCH"
		url = fmt.Sprintf("https://%s/api/v1/notifications/threads/%d", opts.Host, opts.ThreadID)
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusResetContent && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to mark as read (HTTP %d)", resp.StatusCode)
	}

	if opts.All {
		_, _ = fmt.Fprintln(opts.IO.Out, "All notifications marked as read")
	} else {
		_, _ = fmt.Fprintf(opts.IO.Out, "Notification #%d marked as read\n", opts.ThreadID)
	}
	return nil
}
