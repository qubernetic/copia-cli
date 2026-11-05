package cmdutil_test

import (
	"errors"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoWebURL(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		repo  string
		path  string
		want  string
	}{
		{"repository", "my-org", "my-repo", "", "https://app.copia.io/my-org/my-repo"},
		{"subpath", "my-org", "my-repo", "issues/12", "https://app.copia.io/my-org/my-repo/issues/12"},
		{"escaped segments", "my org", "my repo", "issues", "https://app.copia.io/my%20org/my%20repo/issues"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cmdutil.RepoWebURL("app.copia.io", tt.owner, tt.repo, tt.path))
		})
	}
}

func TestOpenInBrowser(t *testing.T) {
	tests := []struct {
		name    string
		tty     bool
		wantErr string
	}{
		{name: "tty", tty: true, wantErr: "Opening app.copia.io/my-org/my-repo/issues in your browser.\n"},
		{name: "no tty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}

			require.NoError(t, cmdutil.OpenInBrowser(ios, b, "https://app.copia.io/my-org/my-repo/issues"))
			assert.Equal(t, []string{"https://app.copia.io/my-org/my-repo/issues"}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
		})
	}
}

func TestOpenInBrowser_Error(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{Err: errors.New("no browser")}
	require.EqualError(t, cmdutil.OpenInBrowser(ios, b, "https://app.copia.io/x/y"), "no browser")
}
