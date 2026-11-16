package prs

import (
	"errors"
	"io"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdSearchPRs(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdSearchPRs_Web(t *testing.T) {
	const (
		dashURL = "https://app.copia.io/pulls"
		orgURL  = "https://app.copia.io/org/my-org/pulls"
		repoURL = "https://app.copia.io/my-org/plc/pulls"
	)
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "dashboard", args: []string{"timeout", "--web"}, wantURL: dashURL + "?q=timeout"},
		{name: "no query", args: []string{"-w"}, wantURL: dashURL},
		{name: "dashboard state", args: []string{"timeout", "-w", "-s", "closed"}, wantURL: dashURL + "?q=timeout&state=closed"},
		{name: "dashboard author login", args: []string{"-w", "--author", "jane"}, wantURL: dashURL + "?poster=jane"},
		{name: "dashboard author me", args: []string{"-w", "--author", "@me"}, wantURL: dashURL + "?type=created_by"},
		{name: "dashboard assignee me", args: []string{"-w", "--assignee", "@me"}, wantURL: dashURL + "?type=assigned"},
		{name: "dashboard assignee login", args: []string{"-w", "--assignee", "jane", "--author", "@me"}, wantURL: dashURL + "?assignee=jane&type=created_by"},
		{
			name: "dashboard author and assignee me", args: []string{"-w", "--author", "@me", "--assignee", "@me"},
			wantURL: dashURL + "?type=created_by",
			wantErr: "warning: --assignee is not applied in the browser\n",
		},
		{name: "owner", args: []string{"sensor", "-w", "--owner", "my-org", "-L", "5"}, wantURL: orgURL + "?q=sensor"},
		{name: "repo", args: []string{"fix", "-w"}, repo: "my-org/plc", wantURL: repoURL + "?q=fix&state=all"},
		{name: "repo without query", args: []string{"-w", "-s", "open"}, repo: "my-org/plc", wantURL: repoURL + "?state=open"},
		{name: "repo and matching owner", args: []string{"-w", "--owner", "MY-ORG"}, repo: "my-org/plc", wantURL: repoURL + "?state=all"},
		{name: "repo author login", args: []string{"-w", "--author", "jane"}, repo: "my-org/plc", wantURL: repoURL + "?poster=jane&state=all"},
		{name: "repo author me", args: []string{"-w", "--author", "@me"}, repo: "my-org/plc", wantURL: repoURL + "?state=all&type=created_by"},
		{name: "repo assignee me", args: []string{"-w", "--assignee", "@me"}, repo: "my-org/plc", wantURL: repoURL + "?state=all&type=assigned"},
		{
			name: "repo assignee login", args: []string{"-w", "--assignee", "jane"}, repo: "my-org/plc",
			wantURL: repoURL + "?state=all",
			wantErr: "warning: --assignee is not applied in the browser\n",
		},
		{
			name: "label", args: []string{"x", "-w", "-l", "bug"},
			wantURL: dashURL + "?q=x",
			wantErr: "warning: --label is not applied in the browser\n",
		},
		{
			name: "tty", args: []string{"x", "-w"}, tty: true,
			wantURL: dashURL + "?q=x",
			wantErr: "Opening app.copia.io/pulls?q=x in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: tt.repo, Browser: b}

			require.NoError(t, runCmd(f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdSearchPRs_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{}
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}

	require.EqualError(t, runCmd(f, "x", "-w", "-s", "merged"), `invalid state "merged": use open or closed`)
	repo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/plc", Browser: b}
	require.EqualError(t, runCmd(repo, "-w", "--owner", "other"), `--owner "other" does not match the owner of --repo "my-org/plc"`)
	badRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "not-a-repo", Browser: b}
	require.Error(t, runCmd(badRepo, "-w"))
	assert.Empty(t, b.URLs)

	noConfig := &cmdutil.Factory{IOStreams: ios, Browser: b,
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(noConfig, "x", "-w"), "no config")

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runCmd(failing, "x", "-w"), "no browser")
	assert.Empty(t, reg.Requests)
}
