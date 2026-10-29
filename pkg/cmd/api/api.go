package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
	// Include prints the status line and response headers before the body.
	Include bool
	// Paginate follows Link rel="next" headers and merges array pages.
	Paginate bool
	// JQ filters each response through a jq expression.
	JQ string
	// Context cancels the requests and the --jq filter; nil means
	// context.Background().
	Context context.Context
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
Use --header to add custom HTTP headers.
Use --include to print the status line and the response headers, sorted by
name, and a blank line before the body.

Use --paginate to fetch every page of a GET request: the next page is taken
from the rel="next" entry of the Link header until there is none. The items
of array pages are printed as one array; any other response is printed as
is. A next page link to another host is refused, so the token stays on the
host it was sent to. --paginate with any method other than GET fails before
the first request.

An HTTP redirect is followed only when it keeps the scheme, host and port
of the request; a redirect to another origin, such as from https to http or
to another port, fails the command, so the token is never sent there.

Use --jq to filter the response with a jq expression, as in "gh api --jq";
with --paginate it runs on each page. Each result is printed on its own
line: a string raw, null as an empty line, anything else as compact JSON.
An error response is printed unfiltered.`,
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

  # Show the status line and headers, for example the total count of a list
  $ copia-cli api -i "/repos/my-org/my-repo/issues?limit=1"

  # List every open issue across all pages, as one JSON array
  $ copia-cli api --paginate "/repos/my-org/my-repo/issues?state=open"

  # Print only the issue titles of every page
  $ copia-cli api --paginate /repos/my-org/my-repo/issues --jq '.[].title'

  # Custom header
  $ copia-cli api /user --header "Accept: application/json"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Path = args[0]
			opts.IO = f.IOStreams
			opts.Context = cmd.Context()

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return APIRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Method, "method", "X", "", "HTTP method (default: GET, or POST with fields or --input)")
	cmd.Flags().StringArrayVarP(&opts.MagicFields, "field", "F", nil, "Add a typed parameter in `key=value` format")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in `key=value` format")
	cmd.Flags().StringSliceVarP(&opts.Headers, "header", "H", nil, "Add HTTP header (key: value)")
	cmd.Flags().StringVar(&opts.InputFile, "input", "", "The `file` to use as the request body (\"-\" for standard input)")
	cmd.Flags().BoolVarP(&opts.Include, "include", "i", false, "Include the HTTP status line and response headers in the output")
	cmd.Flags().BoolVar(&opts.Paginate, "paginate", false, "Make additional GET requests to fetch all pages of results")
	cmd.Flags().StringVarP(&opts.JQ, "jq", "q", "", "Query to select values from the response using jq syntax")

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
	if opts.Paginate && !strings.EqualFold(method, "GET") {
		return errors.New("the `--paginate` option is not supported for non-GET requests")
	}
	if opts.JQ != "" {
		if err := cmdutil.ValidateJQ(opts.JQ); err != nil {
			return err
		}
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
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
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
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

	p := &responsePrinter{opts: opts, ctx: ctx}
	if opts.Paginate {
		err = cmdutil.FollowLinkPages(opts.HTTPClient, req, p.page)
	} else {
		err = sendOnce(opts.HTTPClient, req, p.page)
	}
	p.flush()
	return err
}

// sendOnce sends req and hands the response to page.
func sendOnce(client *http.Client, req *http.Request, page func(*http.Response) error) error {
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", req.URL.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return page(resp)
}

// responsePrinter prints each response of the api command. With --paginate
// it holds the items of array pages back and prints them as one array.
type responsePrinter struct {
	opts    *APIOptions
	ctx     context.Context
	items   []json.RawMessage
	merging bool
}

func (p *responsePrinter) page(resp *http.Response) error {
	w := p.opts.IO.Out
	if p.opts.Include {
		printHead(w, resp)
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		p.flush()
		printBody(w, respBody)
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	if p.opts.JQ != "" {
		dec := json.NewDecoder(bytes.NewReader(respBody))
		dec.UseNumber()
		var data any
		if err := dec.Decode(&data); err != nil {
			return fmt.Errorf("parsing the response for --jq: %w", err)
		}
		return cmdutil.FilterJQ(p.ctx, w, data, p.opts.JQ)
	}

	if p.opts.Paginate && bytes.HasPrefix(bytes.TrimSpace(respBody), []byte("[")) {
		var items []json.RawMessage
		if json.Unmarshal(respBody, &items) == nil {
			p.items = append(p.items, items...)
			p.merging = true
			return nil
		}
	}

	printBody(w, respBody)
	return nil
}

// flush prints the merged array pages, if any.
func (p *responsePrinter) flush() {
	if !p.merging {
		return
	}
	var merged bytes.Buffer
	merged.WriteByte('[')
	for i, item := range p.items {
		if i > 0 {
			merged.WriteByte(',')
		}
		merged.Write(item)
	}
	merged.WriteByte(']')
	printBody(p.opts.IO.Out, merged.Bytes())
	p.items, p.merging = nil, false
}

// printBody prints body indented when it is JSON, as is otherwise.
func printBody(w io.Writer, body []byte) {
	var prettyJSON bytes.Buffer
	if json.Indent(&prettyJSON, body, "", "  ") == nil {
		_, _ = fmt.Fprintln(w, prettyJSON.String())
	} else {
		_, _ = fmt.Fprintln(w, string(body))
	}
}

// printHead prints the status line, the response headers sorted by name and
// a blank line, as gh api --include does.
func printHead(w io.Writer, resp *http.Response) {
	proto := resp.Proto
	if proto == "" {
		proto = "HTTP/1.1"
	}
	status := resp.Status
	if status == "" {
		status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", proto, status)

	names := make([]string, 0, len(resp.Header))
	for name := range resp.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		_, _ = fmt.Fprintf(w, "%s: %s\n", name, strings.Join(resp.Header[name], ", "))
	}
	_, _ = fmt.Fprintln(w)
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
