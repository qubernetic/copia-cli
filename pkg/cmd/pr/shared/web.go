package shared

import (
	"fmt"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// OpenPR opens the web page of the pull request ref in the browser, followed
// by tab (for example "files") when it is not empty. A pull request URL on
// another host than the active one fails, as it does for the API commands.
// It makes no API call, so no token is needed.
func OpenPR(f *cmdutil.Factory, ref PRRef, tab string) error {
	host, err := f.ResolveHost()
	if err != nil {
		return err
	}
	var owner, repo string
	if ref.Host == "" {
		owner, repo, err = f.ResolveRepo()
		if err != nil {
			return err
		}
	}
	owner, repo, err = ref.Resolve(host, owner, repo)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("pulls/%d", ref.Number)
	if tab != "" {
		path += "/" + tab
	}
	return cmdutil.OpenInBrowser(f.IOStreams, f.Browser, cmdutil.RepoWebURL(host, owner, repo, path))
}
