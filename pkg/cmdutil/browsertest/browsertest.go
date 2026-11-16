// Package browsertest provides a fake browser for the tests of the commands
// that take --web.
package browsertest

// Browser is a fake cmdutil.Browser that records the URLs it is asked to
// open and returns Err.
type Browser struct {
	URLs []string
	Err  error
}

// Browse records url and returns b.Err.
func (b *Browser) Browse(url string) error {
	b.URLs = append(b.URLs, url)
	return b.Err
}
