package list

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
	cmd := NewCmdList(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdList_Web(t *testing.T) {
	const labelsURL = "https://app.copia.io/my-org/my-repo/labels"
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "web", args: []string{"--web"}, wantURL: labelsURL},
		{name: "short flag", args: []string{"-w"}, wantURL: labelsURL},
		{name: "repo flag", args: []string{"-w"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/labels"},
		{name: "limit and json ignored", args: []string{"-w", "-L", "5", "--json", "name"}, wantURL: labelsURL},
		{
			name: "tty", args: []string{"-w"}, tty: true,
			wantURL: labelsURL,
			wantErr: "Opening app.copia.io/my-org/my-repo/labels in your browser.\n",
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

func TestNewCmdList_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{}

	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: b,
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(noConfig, "-w"), "no config")

	noRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}
	err := runCmd(noRepo, "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
	assert.Empty(t, b.URLs)

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runCmd(failing, "-w"), "no browser")
	assert.Empty(t, reg.Requests)
}
