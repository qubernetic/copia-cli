package issues

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
	cmd := NewCmdSearchIssues(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdSearchIssues_Web(t *testing.T) {
	const issuesURL = "https://app.copia.io/my-org/my-repo/issues"
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "defaults", args: []string{"timeout", "--web"}, wantURL: issuesURL + "?q=timeout&state=all"},
		{name: "short flag", args: []string{"timeout", "-w"}, wantURL: issuesURL + "?q=timeout&state=all"},
		{name: "repo flag", args: []string{"x", "-w"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/issues?q=x&state=all"},
		{name: "state", args: []string{"sensor timeout", "-w", "-s", "closed"}, wantURL: issuesURL + "?q=sensor+timeout&state=closed"},
		{name: "limit ignored", args: []string{"x", "-w", "-L", "5"}, wantURL: issuesURL + "?q=x&state=all"},
		{
			name: "label", args: []string{"x", "-w", "-l", "bug"},
			wantURL: issuesURL + "?q=x&state=all",
			wantErr: "warning: --label is not applied in the browser\n",
		},
		{
			name: "tty", args: []string{"x", "-w"}, tty: true,
			wantURL: issuesURL + "?q=x&state=all",
			wantErr: "Opening app.copia.io/my-org/my-repo/issues?q=x&state=all in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			repo := tt.repo
			if repo == "" {
				repo = "my-org/my-repo"
			}
			b := &browsertest.Browser{}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: repo, Browser: b}

			require.NoError(t, runCmd(f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdSearchIssues_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{}

	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: b,
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(noConfig, "x", "-w"), "no config")

	noRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}
	err := runCmd(noRepo, "x", "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
	assert.Empty(t, b.URLs)

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runCmd(failing, "x", "-w"), "no browser")
	assert.Empty(t, reg.Requests)
}
