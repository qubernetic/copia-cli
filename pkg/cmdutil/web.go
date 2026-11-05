package cmdutil

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// RepoWebURL returns the web URL of the repository owner/repo on host,
// followed by path (for example "issues/12") when it is not empty.
func RepoWebURL(host, owner, repo, path string) string {
	u := fmt.Sprintf("https://%s/%s/%s", host, url.PathEscape(owner), url.PathEscape(repo))
	if path != "" {
		u += "/" + path
	}
	return u
}

// OpenInBrowser opens u with b. When stdout is a terminal it first prints
// "Opening <url> in your browser." to stderr, as gh does for --web.
func OpenInBrowser(ios *iostreams.IOStreams, b Browser, u string) error {
	if ios.IsStdoutTTY() {
		_, _ = fmt.Fprintf(ios.ErrOut, "Opening %s in your browser.\n", strings.TrimPrefix(u, "https://"))
	}
	return b.Browse(u)
}

// WarnWebIgnored prints a warning to stderr for each flag in flags (names
// without dashes) that is set but that the web page cannot take.
func WarnWebIgnored(ios *iostreams.IOStreams, flags ...string) {
	for _, name := range flags {
		_, _ = fmt.Fprintf(ios.ErrOut, "warning: --%s is not applied in the browser\n", name)
	}
}
