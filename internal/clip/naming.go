package clip

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/krismp/highlighter/internal/timecode"
)

// Filename is "NN_m-ss[_slug].mp4"; index is 1-based play order.
func Filename(index int, c Clip) string {
	name := fmt.Sprintf("%02d_%s", index, strings.ReplaceAll(timecode.Format(c.Start), ":", "-"))
	if s := Slug(c.Label); s != "" {
		name += "_" + s
	}
	return name + ".mp4"
}

// Slug lowercases s, keeps letters and digits and joins everything else with
// single dashes, capped at 40 runes.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := []rune(strings.TrimSuffix(b.String(), "-"))
	if len(out) > 40 {
		out = out[:40]
	}
	return strings.TrimSuffix(string(out), "-")
}

// SafeDirName turns a video title into a folder name safe on macOS.
func SafeDirName(title string) string {
	mapped := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || unicode.IsControl(r) {
			return '-'
		}
		return r
	}, title)
	mapped = strings.Trim(strings.TrimSpace(mapped), ".")
	mapped = strings.TrimSpace(mapped)
	if runes := []rune(mapped); len(runes) > 80 {
		mapped = strings.TrimSpace(string(runes[:80]))
	}
	if mapped == "" {
		return "untitled"
	}
	return mapped
}
