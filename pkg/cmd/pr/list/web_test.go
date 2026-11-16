package list

import (
	"errors"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList_Web(t *testing.T) {
	const base = "https://app.copia.io/my-org/my-repo/pulls"
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "default state", args: []string{"--web"}, wantURL: base + "?state=open"},
		{name: "short flag", args: []string{"-w", "-s", "closed"}, wantURL: base + "?state=closed"},
		{name: "all", args: []string{"-w", "--state", "all"}, wantURL: base + "?state=all"},
		{
			name: "merged", args: []string{"-w", "--state", "merged"}, wantURL: base + "?state=closed",
			wantErr: "warning: --state merged is not applied in the browser, showing closed pull requests\n",
		},
		{name: "ignores limit and json", args: []string{"-w", "-L", "5", "--json", "number"}, wantURL: base + "?state=open"},
		{name: "repo flag", args: []string{"-w"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/pulls?state=open"},
		{
			name: "tty", args: []string{"-w"}, tty: true, wantURL: base + "?state=open",
			wantErr: "Opening app.copia.io/my-org/my-repo/pulls?state=open in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_TOKEN", "")
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			repo := tt.repo
			if repo == "" {
				repo = "my-org/my-repo"
			}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: repo, Browser: b}

			require.NoError(t, runList(t, f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdList_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()

	b := &browsertest.Browser{}
	ready := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo", Browser: b}
	require.EqualError(t, runList(t, ready, "-w", "-s", "draft"), `invalid state "draft": valid values are {open|closed|merged|all}`)
	assert.Empty(t, b.URLs)

	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: &browsertest.Browser{},
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runList(t, noConfig, "-w"), "no config")

	noRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: &browsertest.Browser{}}
	err := runList(t, noRepo, "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runList(t, failing, "-w"), "no browser")
}
