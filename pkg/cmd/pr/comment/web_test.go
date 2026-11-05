package comment

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

func TestNewCmdComment_Web(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "long flag", args: []string{"7", "--web"}, wantURL: "https://app.copia.io/my-org/my-repo/pulls/7"},
		{name: "short flag", args: []string{"#7", "-w"}, wantURL: "https://app.copia.io/my-org/my-repo/pulls/7"},
		{name: "repo flag", args: []string{"3", "-w"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/pulls/3"},
		{name: "url argument", args: []string{"https://app.copia.io/other/thing/pulls/5", "-w"}, wantURL: "https://app.copia.io/other/thing/pulls/5"},
		{
			name: "tty", args: []string{"7", "-w"}, tty: true,
			wantURL: "https://app.copia.io/my-org/my-repo/pulls/7",
			wantErr: "Opening app.copia.io/my-org/my-repo/pulls/7 in your browser.\n",
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

			require.NoError(t, runCmd(t, f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdComment_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()

	for _, args := range [][]string{
		{"7", "-w", "-b", "x"},
		{"7", "-w", "-F", "-"},
		{"7", "-w", "-e"},
	} {
		err := runCmd(t, &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo", Browser: &browsertest.Browser{}}, args...)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "none of the others can be")
	}

	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: &browsertest.Browser{},
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(t, noConfig, "7", "-w"), "no config")

	noRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: &browsertest.Browser{}}
	err := runCmd(t, noRepo, "7", "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")

	b := &browsertest.Browser{}
	otherHost := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}
	require.EqualError(t, runCmd(t, otherHost, "https://copia.example.com/other/thing/pulls/5", "-w"),
		"pull request URL host copia.example.com does not match the active host app.copia.io")
	assert.Empty(t, b.URLs)

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runCmd(t, failing, "7", "-w"), "no browser")
}
