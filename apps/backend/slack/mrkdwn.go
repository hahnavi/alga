package slack

import (
	"regexp"
)

var (
	reBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic = regexp.MustCompile(`__(.+?)__`)
	reH3     = regexp.MustCompile(`(?m)^###\s+(.+)$`)
	reH2     = regexp.MustCompile(`(?m)^##\s+(.+)$`)
	reH1     = regexp.MustCompile(`(?m)^#\s+(.+)$`)
)

func Mrkdwn(text string) string {
	s := text

	s = reH3.ReplaceAllString(s, "*$1*")
	s = reH2.ReplaceAllString(s, "*$1*")
	s = reH1.ReplaceAllString(s, "*$1*")
	s = reBold.ReplaceAllString(s, "*$1*")
	s = reItalic.ReplaceAllString(s, "_${1}_")

	return s
}

// MrkdwnPlain converts Markdown to Slack mrkdwn without a sender prefix.
// Useful for agent messages that already include their own formatting.
func MrkdwnPlain(text string) string {
	return Mrkdwn(text)
}

// MrkdwnPrefixed converts Markdown to Slack mrkdwn with a bold sender prefix.
func MrkdwnPrefixed(sender, text string) string {
	return "*" + sender + "*: " + Mrkdwn(text)
}
