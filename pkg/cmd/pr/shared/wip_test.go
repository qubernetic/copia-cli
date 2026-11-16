package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasWIPPrefix(t *testing.T) {
	tests := []struct {
		title string
		want  bool
	}{
		{title: "WIP: feat: x", want: true},
		{title: "wip:feat: x", want: true},
		{title: "Wip: feat: x", want: true},
		{title: "[WIP] feat: x", want: true},
		{title: "[wip] feat: x", want: true},
		{title: "feat: x", want: false},
		{title: "WIPE: x", want: false},
		{title: "WIP feat: x", want: false},
		{title: " WIP: feat: x", want: false},
		{title: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			assert.Equal(t, tt.want, HasWIPPrefix(tt.title))
		})
	}
}

func TestStripWIPPrefix(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{title: "WIP: feat: x", want: "feat: x"},
		{title: "wip:feat: x", want: "feat: x"},
		{title: "[WIP] feat: x", want: "feat: x"},
		{title: "[Wip]feat: x", want: "feat: x"},
		{title: "WIP: [WIP] feat: x", want: "feat: x"},
		{title: "feat: x", want: "feat: x"},
		{title: "WIPE: x", want: "WIPE: x"},
		{title: "WIP:", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			assert.Equal(t, tt.want, StripWIPPrefix(tt.title))
		})
	}
}
