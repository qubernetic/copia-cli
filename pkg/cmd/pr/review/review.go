package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type ReviewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Event      string
	Body       string
}

type reviewRequest struct {
	Event string `json:"event"`
	Body  string `json:"body,omitempty"`
}

func NewCmdReview(f *cmdutil.Factory) *cobra.Command {
	opts := &ReviewOptions{}

	cmd := &cobra.Command{
		Use:   "review <number>",
		Short: "Submit a review on a pull request",
		Long:  "Add a review to a pull request. Use --approve, --request-changes, or --comment to specify the review action.",
		Example: `  # Approve a pull request
  $ copia-cli pr review 7 --approve

  # Request changes
  $ copia-cli pr review 7 --request-changes --body "Please fix the tests."

  # Leave a comment
  $ copia-cli pr review 7 --comment --body "Looks good overall."`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			num, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid PR number: %s", args[0])
			}
			opts.Number = num
			opts.IO = f.IOStreams

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

			approve, _ := cmd.Flags().GetBool("approve")
			requestChanges, _ := cmd.Flags().GetBool("request-changes")
			comment, _ := cmd.Flags().GetBool("comment")

			switch {
			case approve:
				opts.Event = "APPROVED"
			case requestChanges:
				opts.Event = "REQUEST_CHANGES"
			case comment:
				opts.Event = "COMMENT"
			default:
				return fmt.Errorf("specify --approve, --request-changes, or --comment")
			}

			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ReviewRun(opts)
		},
	}

	cmd.Flags().Bool("approve", false, "Approve the PR")
	cmd.Flags().Bool("request-changes", false, "Request changes")
	cmd.Flags().Bool("comment", false, "Leave a review comment")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Review body text")

	return cmd
}

func ReviewRun(opts *ReviewOptions) error {
	payload := reviewRequest{
		Event: opts.Event,
		Body:  opts.Body,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d/reviews",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to submit review (HTTP %d)", resp.StatusCode)
	}

	action := "Reviewed"
	switch opts.Event {
	case "APPROVED":
		action = "Approved"
	case "REQUEST_CHANGES":
		action = "Requested changes on"
	case "COMMENT":
		action = "Commented on"
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "%s PR #%d\n", action, opts.Number)
	return nil
}
