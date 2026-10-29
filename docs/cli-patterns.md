# GitHub CLI Architecture Patterns

Reference implementations extracted from `github.com/cli/cli` for Copia CLI development.

## 1. Minimal Entrypoint: cmd/gh/main.go

```go
package main

import (
	"os"

	"github.com/cli/cli/v2/internal/ghcmd"
)

func main() {
	code := ghcmd.Main()
	os.Exit(int(code))
}
```

**Pattern:** Delegates to `ghcmd.Main()` which handles full initialization. Exit code returned as integer.

---

## 2. Factory Dependency Injection: pkg/cmdutil/factory.go

```go
type Factory struct {
	AppVersion     string
	ExecutableName string
	InvokingAgent  string

	Browser          browser.Browser
	ExtensionManager extensions.ExtensionManager
	GitClient        *git.Client
	IOStreams        *iostreams.IOStreams
	Prompter         prompter.Prompter

	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	
	// Specialized HTTP client for low-level requests
	PlainHttpClient func() (*http.Client, error)
	Remotes         func() (context.Remotes, error)
}

// Executable is the path to the currently invoked binary
func (f *Factory) Executable() string {
	ghPath := os.Getenv("GH_PATH")
	if ghPath != "" {
		return ghPath
	}
	if !strings.ContainsRune(f.ExecutableName, os.PathSeparator) {
		f.ExecutableName = executable(f.ExecutableName)
	}
	return f.ExecutableName
}
```

**Pattern:** Factory holds all dependencies as fields. Functions that return dependencies allow lazy initialization and mocking. Each command receives factory instance and extracts what it needs.

---

## 3. IOStreams Definition: pkg/iostreams/iostreams.go

```go
type IOStreams struct {
	term term  // terminal abstraction

	In     fileReader   // stdin
	Out    fileWriter   // stdout
	ErrOut fileWriter   // stderr

	terminalTheme string

	progressIndicatorEnabled bool
	progressIndicator        *spinner.Spinner
	progressIndicatorMu      sync.Mutex
	spinnerDisabled          bool

	alternateScreenBufferEnabled bool
	alternateScreenBufferActive  bool
	alternateScreenBufferMu      sync.Mutex

	stdoutIsTTY bool
	stderrIsTTY bool
	stdinIsTTY  bool

	colorOverride        string
	colorEnabled         bool
	colorLabels          bool
	accessibleColorsEnabled bool

	pagerCommand string
	pagerProcess *os.Process

	neverPrompt                 bool
	accessiblePrompterEnabled   bool
	experimentalPrompterEnabled bool

	TempFileOverride *os.File
}

// Factory method for testing
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	io := &IOStreams{
		In: &fdReader{
			fd:         0,
			ReadCloser: io.NopCloser(in),
		},
		Out:    &fdWriter{fd: 1, Writer: out},
		ErrOut: &fdWriter{fd: 2, Writer: errOut},
		term:   &fakeTerm{},
	}
	io.SetStdinTTY(false)
	io.SetStdoutTTY(false)
	io.SetStderrTTY(false)
	return io, in, out, errOut
}
```

**Pattern:** Wraps stdin/stdout/stderr with metadata. Provides `Test()` factory for creating mock IO in tests. Internal `term` interface abstracts terminal detection.

---

## 4. Config Package Structure: internal/config/

Key types (from migration tests):

```go
// Config wraps github.com/cli/go-gh/v2/pkg/config
// Main config file: ~/.config/gh/config.yml
type Config interface {
	Keys(keyPath ...string) ([]string, error)
	Get(key ...string) (string, error)
	Set(key []string, value string) error
	Remove(keyPath ...string) error
}

// Hosts configuration
// File: ~/.config/gh/hosts.yml
// Structure:
// hosts:
//   github.com:
//     user: username
//     oauth_token: token
//     git_protocol: ssh|https
//   enterprise.com:
//     user: username2
//     oauth_token: token2
//     git_protocol: https
```

**Pattern:** Config is interface-based. Multiple config files (main + hosts). Tests use `StubWriteConfig()` helper.

---

## 5. Complete List Command: pkg/cmd/issue/list/list.go

```go
type ListOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	Assignee     string
	Labels       []string
	State        string
	LimitResults int
	Author       string
	Mention      string
	Milestone    string
	Search       string
	WebMode      bool
	Exporter     cmdutil.Exporter

	Detector fd.Detector
	Now      func() time.Time
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
		Now:        time.Now,
	}

	var appAuthor string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues in a repository",
		Long: heredoc.Doc(`
			List issues in a repository.

			By default, shows open issues sorted by recently updated.
		`),
		Example: heredoc.Doc(`
			$ gh issue list
			$ gh issue list --author monalisa
			$ gh issue list --assignee "@me"
			$ gh issue list --milestone "The big 1.0"
			$ gh issue list --search "error no:assignee sort:created-asc"
			$ gh issue list --state all
		`),
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo OWNER/REPO` flag
			opts.BaseRepo = f.BaseRepo
			opts.HttpClient = f.HttpClient
			opts.Config = f.Config
			opts.Browser = f.Browser
			
			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	// Flag definitions
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Filter by assignee")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by labels")
	cmd.Flags().StringVarP(&opts.Author, "author", "", "", "Filter by author")
	cmd.Flags().StringVarP(&opts.Mention, "mention", "", "", "Filter by mention")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Filter by milestone")
	cmd.Flags().StringVarP(&opts.Search, "search", "S", "", "Search issues by keywords")
	cmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number to fetch")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open in browser")
	cmd.Flags().StringVar(&appAuthor, "app", "", "Filter by app name")

	cmdutil.AddFormatFlags(cmd, &opts.Exporter, exporter.IssueFields)

	return cmd
}

var defaultFields = []string{
	"number",
	"title",
	"url",
	"state",
	"updatedAt",
	"labels",
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	// Normalize state for query
	issueState := strings.ToLower(opts.State)
	if issueState == "open" && prShared.QueryHasStateClause(opts.Search) {
		issueState = ""
	}

	// Initialize feature detector (caches for 24h)
	if opts.Detector == nil {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		opts.Detector = fd.NewDetector(cachedClient, baseRepo.RepoHost())
	}

	fields := append(defaultFields, "stateReason")

	filterOptions := prShared.FilterOptions{
		Entity:    "issue",
		State:     issueState,
		Assignee:  opts.Assignee,
		Labels:    opts.Labels,
		Author:    opts.Author,
		Mention:   opts.Mention,
		Milestone: opts.Milestone,
		Search:    opts.Search,
		Fields:    fields,
	}

	isTerminal := opts.IO.IsStdoutTTY()

	if opts.WebMode {
		// Open in browser
		issueListURL := ghrepo.GenerateRepoURL(baseRepo, "issues")
		return opts.Browser.Browse(issueListURL)
	}

	// GraphQL query execution
	// (actual query building omitted for brevity)
	
	return nil
}
```

**Pattern:** 
- Options struct holds all parameters + injected dependencies
- `NewCmdList()` creates Cobra command, initializes Options with factory defaults
- Flags mapped to Options fields with `cmd.Flags().VarP()`
- `RunE` closure re-injects dependencies before calling `listRun()`
- `listRun()` is testable pure function that doesn't reference Cobra or CLI layer

---

## 6. Parent Command Grouping: pkg/cmd/issue/issue.go

```go
func NewCmdIssue(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <command>",
		Short: "Manage issues",
		Long:  `Work with GitHub issues.`,
		Example: heredoc.Doc(`
			$ gh issue list
			$ gh issue create --label bug
			$ gh issue view 123 --web
		`),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				An issue can be supplied as argument in any of the following formats:
				- by number, e.g. "123"; or
				- by URL, e.g. "https://github.com/OWNER/REPO/issues/123".
			`),
		},
		GroupID: "core",
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmdutil.AddGroup(cmd, "General commands",
		cmdList.NewCmdList(f, nil),
		cmdCreate.NewCmdCreate(f, nil),
		cmdStatus.NewCmdStatus(f, nil),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		cmdView.NewCmdView(f, nil),
		cmdComment.NewCmdComment(f, nil),
		cmdClose.NewCmdClose(f, nil),
		cmdReopen.NewCmdReopen(f, nil),
		cmdEdit.NewCmdEdit(f, nil),
		cmdDevelop.NewCmdDevelop(f, nil),
		cmdLock.NewCmdLock(f, cmd.Name(), nil),
		cmdLock.NewCmdUnlock(f, cmd.Name(), nil),
		cmdPin.NewCmdPin(f, nil),
		cmdUnpin.NewCmdUnpin(f, nil),
		cmdTransfer.NewCmdTransfer(f, nil),
		cmdDelete.NewCmdDelete(f, nil),
	)

	return cmd
}
```

**Pattern:** 
- Parent command uses `cmdutil.AddGroup()` to organize subcommands into logical groups
- Groups displayed separately in help text
- Each subcommand passed factory instance `f`
- `EnableRepoOverride()` adds `-R, --repo` flag inheritance

---

## 7. HTTP Mock for Testing: pkg/httpmock/httpmock.go

```go
type Registry struct {
	// internal mocking state
}

// Basic pattern usage:
func TestExample(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)  // Verify all registered mocks were called

	// Register GraphQL query handler
	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.GraphQLQuery(`
		{ "data": {	"repository": {
			"hasIssuesEnabled": true,
			"issues": { "nodes": [] }
		} } }`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, "expected-value", params["author"].(string))
		}))

	// Register REST handler with file response
	http.Register(
		httpmock.GraphQL(`query Something\b`),
		httpmock.FileResponse("./fixtures/response.json"))

	// Register handler with string response
	http.Register(
		httpmock.GraphQL(`query Other\b`),
		httpmock.StringResponse(`{ "data": {...} }`),
	)

	// Use in test: pass http as RoundTripper to HTTP client
	client := &http.Client{Transport: http}
}
```

**Pattern:** 
- `Registry` is `http.RoundTripper` implementation
- `Register(matcher, handler)` - matcher detects request type, handler returns response
- `Verify(t)` ensures all registered mocks were called exactly once
- Supports GraphQL, REST, file fixtures
- Handlers can assert on parameters

---

## 8. Command Test Structure: pkg/cmd/issue/list/list_test.go

```go
func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	// Setup IO streams with test buffers
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	// Create factory with mocked dependencies
	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	fakeNow := func() time.Time {
		return time.Date(2022, time.August, 25, 23, 50, 0, 0, time.UTC)
	}

	// Build command and parse arguments
	cmd := NewCmdList(factory, func(opts *ListOptions) error {
		opts.Now = fakeNow
		return listRun(opts)
	})

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

// Test examples
func TestIssueList_nontty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.FileResponse("./fixtures/issueList.json"))

	output, err := runCommand(http, false, "")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	assert.Equal(t, "", output.Stderr())
	test.ExpectLines(t, output.String(),
		`1[\t]+number won[\t]+label[\t]+\d+`,
		`2[\t]+number too[\t]+label[\t]+\d+`,
		`4[\t]+number fore[\t]+label[\t]+\d+`)
}

func TestIssueList_tty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.FileResponse("./fixtures/issueList.json"))

	output, err := runCommand(http, true, "")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	assert.Equal(t, heredoc.Doc(`
		Showing 3 of 3 open issues in OWNER/REPO

		ID  TITLE        LABELS  UPDATED
		#1  number won   label   about 1 day ago
		#2  number too   label   about 1 month ago
		#4  number fore  label   about 2 years ago
	`), output.String())
	assert.Equal(t, ``, output.Stderr())
}

func TestIssueList_withInvalidLimitFlag(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	_, err := runCommand(http, true, "--limit=0")

	if err == nil || err.Error() != "invalid limit: 0" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}
```

**Pattern:**
- `runCommand()` test helper creates Factory with mocked IO, config, HTTP
- `iostreams.Test()` returns IOStreams with captured buffers
- Separate TTY vs non-TTY tests to verify output formatting
- Flag validation tested separately
- Assertions use regex matching for flexible output verification

---

## 9. Version Injection: internal/build/build.go

```go
package build

import (
	"os"
	"runtime/debug"
)

// Version is dynamically set by the toolchain or overridden by the Makefile.
var Version = "DEV"

// Date is dynamically set at build time in the Makefile.
var Date = "" // YYYY-MM-DD

func init() {
	if Version == "DEV" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
}
```

**Pattern:** Version variables set to default, overridden at link time via `-ldflags`. Runtime fallback to `debug.ReadBuildInfo()` for development builds.

---

## 10. Makefile Key Targets

```makefile
CGO_CPPFLAGS ?= ${CPPFLAGS}
export CGO_CPPFLAGS
CGO_CFLAGS ?= ${CFLAGS}
export CGO_CFLAGS
CGO_LDFLAGS ?= $(filter -g -L% -l% -O%,${LDFLAGS})
export CGO_LDFLAGS

EXE =
ifeq ($(shell go env GOOS),windows)
EXE = .exe
endif

# Build
build:
	script/build.go

# Tests
test:
	go test -v ./...

# Linting
lint:
	golangci-lint run ./...

# Code generation
generate:
	go generate ./...
```

**Pattern:** Delegates heavy lifting to Go scripts (`script/build.go`). Cross-platform via environment detection.

---

## 11. Key go.mod Dependencies

```
require (
	charm.land/bubbles/v2 v2.0.0          # TUI components
	charm.land/bubbletea/v2 v2.0.2        # TUI framework
	charm.land/huh/v2 v2.0.3              # Interactive forms
	charm.land/lipgloss/v2 v2.0.2         # Terminal styling
	github.com/AlecAivazis/survey/v2 v2.3.7       # User prompts
	github.com/cli/go-gh/v2 v2.13.0       # Shared CLI utilities
	github.com/cli/go-internal v0.0.0-...         # Internal utilities
	github.com/cli/oauth v1.2.2           # OAuth flow
	github.com/cpuguy83/go-md2man/v2 v2.0.7      # Markdown to man pages
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510  # Argument parsing
	github.com/spf13/cobra v1.x.x         # CLI framework
	github.com/stretchr/testify v1.x.x    # Testing assertions
	github.com/shurcooL/githubv4 v0.x.x   # GraphQL client
)
```

**Pattern:** Mix of TUI (Bubble Tea), CLI (Cobra), HTTP (net/http + custom), GraphQL, and testing libraries.

---

## 12. GoReleaser Configuration: .goreleaser.yml

```yaml
version: 2
project_name: gh
release:
  prerelease: auto
  draft: true
  name_template: "GitHub CLI {{.Version}}"

before:
  hooks:
    - make manpages GH_VERSION={{.Version}}
    - make completions
    - pwsh '.\script\gen-winres.ps1' '{{ .Version }}' ...

builds:
  - id: macos
    goos: [darwin]
    goarch: [amd64, arm64]
    binary: bin/gh
    main: ./cmd/gh
    ldflags:
      - -s -w 
      - -X github.com/cli/cli/v2/internal/build.Version={{.Version}}
      - -X github.com/cli/cli/v2/internal/build.Date={{time "2006-01-02"}}

  - id: linux
    goos: [linux]
    goarch: ["386", arm, amd64, arm64]
    env:
      - CGO_ENABLED=0
    binary: bin/gh

  - id: windows
    goos: [windows]
    goarch: [amd64, arm64]
    binary: gh.exe

archives:
  - format: tar.gz
    files:
      - LICENSE
      - README.md
      - completions/*
      - manpages/*

nfpms:
  - id: linux
    package_name: gh
    file_name_template: "{{ .PackageName }}_{{ .Version }}_{{ .Arch }}"
    formats: [deb, rpm]

brews:
  - repository:
      owner: cli
      name: homebrew-gh
```

**Pattern:** 
- Pre-hooks generate manpages, completions, Windows resources
- Separate builds per OS/arch with platform-specific settings
- ldflags inject version and date
- Archives include docs and generated files
- Integrates with Homebrew, apt/rpm

---

## Key Design Principles

1. **Dependency Injection via Factory**: All commands receive factory that provides HTTP, config, IO, etc. Testable with mock factory.

2. **Lazy Initialization**: Dependencies returned via functions, allowing delayed/cached initialization and test override.

3. **Options Struct Pattern**: Each command has dedicated `XxxOptions` struct holding flags + injected deps. Decouples CLI parsing from business logic.

4. **Cobra for CLI Layer**: Handles argument parsing, help text, flag validation. Business logic in pure testable functions.

5. **IOStreams Abstraction**: Wraps stdin/stdout/stderr with TTY detection, color, paging. Enables formatting decisions (table vs JSON vs plain text).

6. **Mock HTTP Registry**: `httpmock.Registry` implements `http.RoundTripper`, registering request matchers and mock responses. Tests use real HTTP client with mocked transport.

7. **Fixture Files**: GraphQL responses stored in `fixtures/` directory as JSON files, loaded via `httpmock.FileResponse()`.

8. **Version at Build Time**: `-ldflags` injects version/date. Fallback to `debug.ReadBuildInfo()` in dev builds.

9. **Help Text Segregation**: `heredoc.Doc()` used for multi-line help/examples. Annotations hold structured metadata (arguments, warnings, etc.).

10. **Command Groups**: Parent commands organize subcommands using `cmdutil.AddGroup()` for better help organization.

---

## For Copia CLI Implementation

Apply these patterns:

- ✓ Create `pkg/cmdutil/factory.go` with Gitea-specific clients
- ✓ Create `pkg/iostreams/iostreams.go` for terminal abstraction
- ✓ Create `cmd/copia-cli/main.go` → `internal/copiacmd/main.go`
- ✓ Each command: `*Options` struct + `NewCmd*()` + `*Run()` function
- ✓ Use `github.com/spf13/cobra` for CLI skeleton
- ✓ Create `pkg/httpmock/` adapter for Gitea API mocking
- ✓ Tests: use `iostreams.Test()`, mock factory, register HTTP mocks
- ✓ Makefile: build via Go script with `-ldflags` for version
- ✓ GoReleaser: multi-arch builds with version injection

### Output flags: `--json`, `--jq` and `--template`

Every command with machine-readable output registers the three flags with one call and prints through the exporter in `pkg/cmdutil/json.go`:

```go
var validJSONFields = []string{"number", "title", "state", "labels", "updated_at"}

cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

// in the Run function
if opts.JSON.IsJSON() {
	return opts.JSON.Print(opts.IO.Out, issues)
}
```

- The valid fields are the JSON keys the command prints (the tags of its output struct), so `--json user` prints the `user` key. Keep the list in step with the struct.
- `--json` takes a comma-separated list and can be repeated. An unknown field fails while the flags are parsed, before any request, with the list of available fields.
- `Print` keeps only the requested keys: an array of objects for a list, one object for a view. Numbers keep every digit (the output is decoded with `json.Number`).
- `--jq/-q` runs the expression with `github.com/itchyny/gojq`, the library `gh` uses, on that filtered output. Each result goes on its own line: a string raw, `null` as an empty line, anything else as compact JSON, as in `gh`. `--jq` without `--json` and an expression that does not parse fail before any request (a `PreRunE` that `AddJSONFlags` adds in front of any existing one).
- `cmdutil.FilterJQ(w, input, expr)` is the `--jq` part on its own, for output that does not come from `--json` (for example `api`).
- `--template/-t` runs a Go `text/template` (`missingkey=zero`) on the same filtered output, as `gh` does, with the `gh` helpers that do not print color, in `pkg/cmdutil/template.go`:
  - `join <sep> <list>` joins the scalars of a list, skipping `null`;
  - `pluck <field> <list>` takes one field of each object in a list;
  - `truncate <width> <string>` cuts to `width` characters, ending in `...` from a width of 5;
  - `timeago <RFC 3339 time>` prints `just now`, `5 minutes ago`, `2 days ago` and so on;
  - `tablerow <field>...` adds a row to a table whose columns are aligned with two spaces, printed by `tablerender` or, without it, at the end.
- `--template` without `--json`, `--template` with `--jq`, and a template that does not parse (an unknown helper included) fail before any request. `cmdutil.ExecuteTemplate(w, data, tpl)` is the `--template` part on its own.

