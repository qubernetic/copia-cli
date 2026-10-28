package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// APIOptions holds all inputs for the api command.
type APIOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Method     string
	Path       string
	// RawFields are -f/--raw-field values, always sent as strings.
	RawFields []string
	// MagicFields are -F/--field values, typed like gh (see magicFieldValue).
	MagicFields []string
	Headers     []string
	// InputFile is sent as the request body; "-" reads stdin.
	InputFile string
}

// NewCmdAPI creates the `copia api` command.
func NewCmdAPI(f *cmdutil.Factory) *cobra.Command {
	opts := &APIOptions{}

	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Make an API request",
		Long: `Make an authenticated HTTP request to the Copia REST API and print the response.

The endpoint argument should be a path of a Gitea API v1 endpoint. The /api/v1
prefix is added automatically if not present.

Parameters are sent as a JSON body, and the default method switches to POST.
Each flag takes key=value and can be repeated; a comma in a value is kept.

  -f/--raw-field sends the value as a string.
  -F/--field types the value like gh:
    - true, false, null and integers are sent as JSON values;
    - @file sends the contents of the file, @- reads standard input;
    - anything else is sent as a string.

Use --input to send a file as the request body ("-" reads standard input);
the default method is then POST, the Content-Type stays application/json
unless --header sets it, and fields are added to the query string instead.

Use --method to override the HTTP method (GET, POST, PATCH, DELETE, PUT).
Use --header to add custom HTTP headers.`,
		Example: `  # Get authenticated user
  $ copia-cli api /user

  # Create an issue
  $ copia-cli api -X POST /repos/my-org/my-repo/issues -f title="Bug report"

  # Create an issue with the body read from a file
  $ copia-cli api /repos/my-org/my-repo/issues -f title="Bug report" -F body=@report.md

  # Typed fields: a number and a boolean
  $ copia-cli api -X PATCH /repos/my-org/my-repo/issues/12 -F milestone=3
  $ copia-cli api /user/repos -f name=demo -F private=true

  # Send a prepared JSON body from standard input
  $ echo '{"title":"Bug report"}' | copia-cli api /repos/my-org/my-repo/issues --input -

  # Delete a repo
  $ copia-cli api -X DELETE /repos/my-org/old-repo

  # Custom header
  $ copia-cli api /user --header "Accept: application/json"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Path = args[0]
			opts.IO = f.IOStreams

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = &http.Client{}
			return APIRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Method, "method", "X", "", "HTTP method (default: GET, or POST with fields or --input)")
	cmd.Flags().StringArrayVarP(&opts.MagicFields, "field", "F", nil, "Add a typed parameter in `key=value` format")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in `key=value` format")
	cmd.Flags().StringSliceVarP(&opts.Headers, "header", "H", nil, "Add HTTP header (key: value)")
	cmd.Flags().StringVar(&opts.InputFile, "input", "", "The `file` to use as the request body (\"-\" for standard input)")

	return cmd
}

func APIRun(opts *APIOptions) error {
	if opts.Path == "" {
		return fmt.Errorf("path required")
	}

	// Normalize path
	path := opts.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasPrefix(path, "/api/v1") {
		path = "/api/v1" + path
	}

	params, err := parseFields(opts)
	if err != nil {
		return err
	}

	// Determine method
	method := opts.Method
	if method == "" {
		if len(params) > 0 || opts.InputFile != "" {
			method = "POST"
		} else {
			method = "GET"
		}
	}

	// Build body from --input, or from fields; with --input, fields go
	// into the query string, as in gh.
	var body io.Reader
	if opts.InputFile != "" {
		b, err := cmdutil.ReadFile(opts.InputFile, opts.IO.In)
		if err != nil {
			return fmt.Errorf("reading --input: %w", err)
		}
		body = bytes.NewReader(b)
		path = addQuery(path, params)
	} else if len(params) > 0 {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	reqURL := fmt.Sprintf("https://%s%s", opts.Host, path)
	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Custom headers
	for _, h := range opts.Headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Pretty-print JSON if possible
	var prettyJSON bytes.Buffer
	if json.Indent(&prettyJSON, respBody, "", "  ") == nil {
		_, _ = fmt.Fprintln(opts.IO.Out, prettyJSON.String())
	} else {
		_, _ = fmt.Fprintln(opts.IO.Out, string(respBody))
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	return nil
}

// parseFields collects -f and -F values into one parameter map. A value
// without "=" is skipped, as before.
func parseFields(opts *APIOptions) (map[string]any, error) {
	params := map[string]any{}
	for _, f := range opts.RawFields {
		if key, value, ok := strings.Cut(f, "="); ok {
			params[key] = value
		}
	}
	for _, f := range opts.MagicFields {
		key, value, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		v, err := magicFieldValue(value, opts)
		if err != nil {
			return nil, fmt.Errorf("error parsing %q value: %w", key, err)
		}
		params[key] = v
	}
	return params, nil
}

// addQuery appends params to the query string of path; null is sent as an
// empty value.
func addQuery(path string, params map[string]any) string {
	if len(params) == 0 {
		return path
	}
	q := url.Values{}
	for key, value := range params {
		if value == nil {
			q.Add(key, "")
		} else {
			q.Add(key, fmt.Sprint(value))
		}
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + q.Encode()
}

// magicFieldValue types a -F value like gh: @file reads a file (@- reads
// stdin), integers, true, false and null become JSON values, anything else
// stays a string.
// Adapted from github.com/cli/cli/v2/pkg/cmd/api.magicFieldValue.
func magicFieldValue(v string, opts *APIOptions) (any, error) {
	if filename, ok := strings.CutPrefix(v, "@"); ok {
		b, err := cmdutil.ReadFile(filename, opts.IO.In)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	return v, nil
}
