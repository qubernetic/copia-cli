package cmdutil

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestReadAPIMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "a message", body: `{"message":"Not Found","url":"x"}`, want: "Not Found"},
		{name: "no message", body: `{}`, want: ""},
		{name: "not JSON", body: `404 page not found`, want: ""},
		{name: "empty", body: ``, want: ""},
		{name: "a long message is cut", body: `{"message":"` + strings.Repeat("m", 10<<10) + `"}`, want: strings.Repeat("m", MaxAPIMessage)},
		// The body is read up to its limit only, so a message past it is
		// not decoded at all.
		{name: "a body past the limit", body: `{"message":"x","pad":"` + strings.Repeat("p", MaxErrorBody) + `"}`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ReadAPIMessage(strings.NewReader(tt.body)))
		})
	}
}

func TestReadAPIMessage_CutsOnARuneBoundary(t *testing.T) {
	// "é" is two bytes, so the cap falls inside the last rune.
	got := ReadAPIMessage(strings.NewReader(`{"message":"a` + strings.Repeat("é", MaxAPIMessage) + `"}`))
	assert.LessOrEqual(t, len(got), MaxAPIMessage)
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, MaxAPIMessage-1, len(got))
}
