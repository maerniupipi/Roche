package service

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Kept variable so tests and future runtime configuration can tune the
// threshold without changing the summary pipeline.
var minTextContentRunes = 10

var (
	mdImageRefRE         = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	imageOriginalBlockRE = regexp.MustCompile(`(?is)<image_original\b[^>]*>.*?</image_original>`)
	htmlImgTagRE         = regexp.MustCompile(`(?i)<img\b[^>]*/?>`)
	imageWrapperTagRE    = regexp.MustCompile(`(?i)</?image[a-z_]*\b[^>]*/?>`)
)

func previewText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "...(truncated)"
}

func realTextRuneCount(content string) int {
	content = imageOriginalBlockRE.ReplaceAllString(content, "")
	content = mdImageRefRE.ReplaceAllString(content, "")
	content = htmlImgTagRE.ReplaceAllString(content, "")
	content = imageWrapperTagRE.ReplaceAllString(content, "")
	return utf8.RuneCountInString(strings.TrimSpace(content))
}
