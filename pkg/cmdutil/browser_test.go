package cmdutil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSystemBrowser_Browse(t *testing.T) {
	const url = "https://app.copia.io/my-org/my-repo"

	tests := []struct {
		name     string
		goos     string
		env      map[string]string
		config   string
		wantName string
		wantArgs []string
	}{
		{
			name:     "linux default",
			goos:     "linux",
			wantName: "xdg-open",
			wantArgs: []string{url},
		},
		{
			name:     "macos default",
			goos:     "darwin",
			wantName: "open",
			wantArgs: []string{url},
		},
		{
			name:     "windows default",
			goos:     "windows",
			wantName: "rundll32",
			wantArgs: []string{"url.dll,FileProtocolHandler", url},
		},
		{
			name:     "BROWSER with arguments",
			goos:     "linux",
			env:      map[string]string{"BROWSER": "firefox --new-tab"},
			wantName: "firefox",
			wantArgs: []string{"--new-tab", url},
		},
		{
			name:     "COPIA_BROWSER beats BROWSER",
			goos:     "darwin",
			env:      map[string]string{"COPIA_BROWSER": "lynx", "BROWSER": "firefox"},
			wantName: "lynx",
			wantArgs: []string{url},
		},
		{
			name:     "config browser beats BROWSER",
			goos:     "linux",
			env:      map[string]string{"BROWSER": "firefox"},
			config:   "chromium --incognito",
			wantName: "chromium",
			wantArgs: []string{"--incognito", url},
		},
		{
			name:     "COPIA_BROWSER beats config browser",
			goos:     "linux",
			env:      map[string]string{"COPIA_BROWSER": "lynx", "BROWSER": "firefox"},
			config:   "chromium",
			wantName: "lynx",
			wantArgs: []string{url},
		},
		{
			name:     "config browser beats system opener",
			goos:     "darwin",
			config:   "chromium",
			wantName: "chromium",
			wantArgs: []string{url},
		},
		{
			name:     "blank COPIA_BROWSER falls through",
			goos:     "linux",
			env:      map[string]string{"COPIA_BROWSER": "  ", "BROWSER": "firefox"},
			wantName: "firefox",
			wantArgs: []string{url},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotName string
			var gotArgs []string
			b := &systemBrowser{
				goos:   tt.goos,
				getenv: func(k string) string { return tt.env[k] },
				config: func(k string) string {
					if k == "browser" {
						return tt.config
					}
					return ""
				},
				run: func(name string, args ...string) error {
					gotName, gotArgs = name, args
					return nil
				},
			}

			assert.NoError(t, b.Browse(url))
			assert.Equal(t, tt.wantName, gotName)
			assert.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}

func TestSystemBrowser_Browse_RunError(t *testing.T) {
	b := &systemBrowser{
		goos:   "linux",
		getenv: func(string) string { return "" },
		config: func(string) string { return "" },
		run:    func(string, ...string) error { return errors.New("exec: \"xdg-open\": not found") },
	}

	err := b.Browse("https://app.copia.io")
	assert.EqualError(t, err, "opening the browser: exec: \"xdg-open\": not found")
}

func TestNewBrowser_UsesSystemDefaults(t *testing.T) {
	b, ok := NewBrowser(func(key string) string { return "value of " + key }).(*systemBrowser)
	if assert.True(t, ok) {
		assert.NotEmpty(t, b.goos)
		assert.NotNil(t, b.getenv)
		assert.Equal(t, "value of browser", b.config("browser"))
		assert.NotNil(t, b.run)
	}
}

func TestStartCommand_MissingCommand(t *testing.T) {
	assert.Error(t, startCommand("copia-cli-test-no-such-browser"))
}
