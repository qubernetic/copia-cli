package shared

import (
	"fmt"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// OpenIssue opens the web page of issue number in the browser. It makes no
// API call, so no token is needed.
func OpenIssue(f *cmdutil.Factory, number int64) error {
	host, err := f.ResolveHost()
	if err != nil {
		return err
	}
	owner, repo, err := f.ResolveRepo()
	if err != nil {
		return err
	}
	return cmdutil.OpenInBrowser(f.IOStreams, f.Browser, cmdutil.RepoWebURL(host, owner, repo, fmt.Sprintf("issues/%d", number)))
}
