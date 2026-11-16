package cmdutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHost(t *testing.T) {
	tests := []struct {
		host    string
		wantErr bool
	}{
		{host: "app.copia.io"},
		{host: "copia.example.com:8443"},
		{host: "[::1]:8443"},
		{host: "", wantErr: true},
		{host: "*.copia.io", wantErr: true},
		{host: "user@app.copia.io", wantErr: true},
		{host: "app.copia.io/path", wantErr: true},
		{host: "app.copia.io?x=1", wantErr: true},
		{host: "app.copia.io#x", wantErr: true},
		{host: "app copia.io", wantErr: true},
		{host: "https://app.copia.io", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			u, err := ParseHost(tt.host)
			if tt.wantErr {
				assert.EqualError(t, err, `invalid host "`+tt.host+`": want a host name with an optional port`)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.host, u.Host)
		})
	}
}

func TestMatchesHost(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		url        string
		want       bool
	}{
		{name: "same host", configured: "app.copia.io", url: "https://app.copia.io/o/r.git", want: true},
		{name: "case-insensitive", configured: "App.Copia.IO", url: "https://app.copia.io", want: true},
		{name: "explicit 443", configured: "app.copia.io", url: "https://app.copia.io:443/o/r", want: true},
		{name: "configured 443", configured: "app.copia.io:443", url: "https://app.copia.io/o/r", want: true},
		{name: "custom port", configured: "copia.example.com:8443", url: "https://copia.example.com:8443", want: true},
		{name: "other port", configured: "app.copia.io", url: "https://app.copia.io:8443"},
		{name: "missing custom port", configured: "copia.example.com:8443", url: "https://copia.example.com"},
		{name: "http", configured: "app.copia.io", url: "http://app.copia.io"},
		{name: "ssh", configured: "app.copia.io", url: "ssh://git@app.copia.io/o/r.git"},
		{name: "other host", configured: "app.copia.io", url: "https://evil.example.com"},
		{name: "suffix host", configured: "app.copia.io", url: "https://app.copia.io.evil.example.com"},
		{name: "unparsable", configured: "app.copia.io", url: "https://app.copia.io:port"},
		{name: "unicode case fold of sigma", configured: "xσ.example", url: "https://xς.example"},
		{name: "kelvin sign for k", configured: "kspace.example", url: "https://Kspace.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured, err := ParseHost(tt.configured)
			require.NoError(t, err)
			assert.Equal(t, tt.want, MatchesHost(configured, tt.url))
		})
	}
}

func TestIsPrintableASCII(t *testing.T) {
	assert.True(t, IsPrintableASCII(""))
	assert.True(t, IsPrintableASCII(" abc~XYZ019!"))
	assert.False(t, IsPrintableASCII("abc\n"))
	assert.False(t, IsPrintableASCII("a\rb"))
	assert.False(t, IsPrintableASCII("a\x00b"))
	assert.False(t, IsPrintableASCII("a\x7fb"))
	assert.False(t, IsPrintableASCII("café"))
}
