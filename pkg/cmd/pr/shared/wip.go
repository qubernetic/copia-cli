package shared

import "strings"

// wipPrefixes are Gitea's default work-in-progress title prefixes. Gitea has
// no draft field: a pull request whose title starts with one of them, in any
// case, is a draft.
var wipPrefixes = []string{"WIP:", "[WIP]"}

// HasWIPPrefix reports whether title starts with one of Gitea's default
// work-in-progress prefixes, which Gitea matches case-insensitively.
func HasWIPPrefix(title string) bool {
	return wipPrefixLen(title) > 0
}

// StripWIPPrefix removes every leading work-in-progress prefix from title,
// with the spaces after it, so Gitea no longer treats the pull request as a
// draft.
func StripWIPPrefix(title string) string {
	for n := wipPrefixLen(title); n > 0; n = wipPrefixLen(title) {
		title = strings.TrimLeft(title[n:], " ")
	}
	return title
}

func wipPrefixLen(title string) int {
	upper := strings.ToUpper(title)
	for _, p := range wipPrefixes {
		if strings.HasPrefix(upper, p) {
			return len(p)
		}
	}
	return 0
}
