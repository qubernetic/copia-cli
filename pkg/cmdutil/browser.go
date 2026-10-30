package cmdutil

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Browser opens a URL in the user's web browser.
type Browser interface {
	Browse(url string) error
}

// NewBrowser returns the Browser that opens a URL with the command in
// COPIA_BROWSER or BROWSER, otherwise with the operating system's opener
// (xdg-open, open, or rundll32 on Windows).
func NewBrowser() Browser {
	return &systemBrowser{goos: runtime.GOOS, getenv: os.Getenv, run: startCommand}
}

type systemBrowser struct {
	goos   string
	getenv func(string) string
	run    func(name string, args ...string) error
}

func (b *systemBrowser) Browse(url string) error {
	name, args := b.command(url)
	if err := b.run(name, args...); err != nil {
		return fmt.Errorf("opening the browser: %w", err)
	}
	return nil
}

func (b *systemBrowser) command(url string) (string, []string) {
	for _, key := range []string{"COPIA_BROWSER", "BROWSER"} {
		if fields := strings.Fields(b.getenv(key)); len(fields) > 0 {
			return fields[0], append(fields[1:], url)
		}
	}
	switch b.goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// startCommand starts the opener without waiting for it, so a browser
// started directly from COPIA_BROWSER does not block the CLI.
func startCommand(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}
