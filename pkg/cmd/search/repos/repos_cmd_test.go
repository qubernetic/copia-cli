package repos

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
	cmd := NewCmdSearchRepos(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdSearchRepos_Web(t *testing.T) {
	const exploreURL = "https://app.copia.io/explore/repos"
	tests := []struct {
		name    string
		args    []string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "query", args: []string{"plc", "--web"}, wantURL: exploreURL + "?q=plc"},
		{name: "short flag", args: []string{"automation controller", "-w"}, wantURL: exploreURL + "?q=automation+controller"},
		{name: "limit ignored", args: []string{"plc", "-w", "-L", "5"}, wantURL: exploreURL + "?q=plc"},
		{name: "order without sort ignored", args: []string{"plc", "-w", "--order", "asc"}, wantURL: exploreURL + "?q=plc"},
		{name: "alpha desc", args: []string{"plc", "-w", "--sort", "alpha"}, wantURL: exploreURL + "?q=plc&sort=reversealphabetically"},
		{name: "alpha asc", args: []string{"plc", "-w", "--sort", "alpha", "--order", "asc"}, wantURL: exploreURL + "?q=plc&sort=alphabetically"},
		{name: "created desc", args: []string{"plc", "-w", "--sort", "created"}, wantURL: exploreURL + "?q=plc&sort=newest"},
		{name: "created asc", args: []string{"plc", "-w", "--sort", "created", "--order", "asc"}, wantURL: exploreURL + "?q=plc&sort=oldest"},
		{name: "updated desc", args: []string{"plc", "-w", "--sort", "updated"}, wantURL: exploreURL + "?q=plc&sort=recentupdate"},
		{name: "updated asc", args: []string{"plc", "-w", "--sort", "updated", "--order", "asc"}, wantURL: exploreURL + "?q=plc&sort=leastupdate"},
		{name: "size desc", args: []string{"plc", "-w", "--sort", "size"}, wantURL: exploreURL + "?q=plc&sort=reversesize"},
		{name: "size asc", args: []string{"plc", "-w", "--sort", "size", "--order", "asc"}, wantURL: exploreURL + "?q=plc&sort=size"},
		{name: "stars desc", args: []string{"plc", "-w", "--sort", "stars"}, wantURL: exploreURL + "?q=plc&sort=moststars"},
		{name: "stars asc", args: []string{"plc", "-w", "--sort", "stars", "--order", "asc"}, wantURL: exploreURL + "?q=plc&sort=feweststars"},
		{name: "forks desc", args: []string{"plc", "-w", "--sort", "forks", "--order", "desc"}, wantURL: exploreURL + "?q=plc&sort=mostforks"},
		{name: "forks asc", args: []string{"plc", "-w", "--sort", "forks", "--order", "asc"}, wantURL: exploreURL + "?q=plc&sort=fewestforks"},
		{
			name: "tty", args: []string{"plc", "-w"}, tty: true,
			wantURL: exploreURL + "?q=plc",
			wantErr: "Opening app.copia.io/explore/repos?q=plc in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}

			require.NoError(t, runCmd(f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdSearchRepos_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{}
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}

	require.EqualError(t, runCmd(f, "plc", "-w", "--sort", "name"),
		`invalid sort "name": valid values are {alpha|created|updated|size|stars|forks}`)
	require.EqualError(t, runCmd(f, "plc", "-w", "--sort", "stars", "--order", "up"),
		`invalid order "up": valid values are {asc|desc}`)
	assert.Empty(t, b.URLs)

	noConfig := &cmdutil.Factory{IOStreams: ios, Browser: b,
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(noConfig, "plc", "-w"), "no config")

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runCmd(failing, "plc", "-w"), "no browser")
	assert.Empty(t, reg.Requests)
}
