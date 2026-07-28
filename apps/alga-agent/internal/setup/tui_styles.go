package setup

import "github.com/charmbracelet/lipgloss"

var (
	tuiAccent  = lipgloss.Color("#A78BFA")
	tuiAccent2 = lipgloss.Color("#67E8F9")
	tuiGreen   = lipgloss.Color("#34D399")
	tuiAmber   = lipgloss.Color("#FBBF24")
	tuiRed     = lipgloss.Color("#F87171")
	tuiFaint   = lipgloss.Color("#4B5563")
	tuiMuted   = lipgloss.Color("#9CA3AF")
	tuiText    = lipgloss.Color("#F3F4F6")
	tuiBg      = lipgloss.Color("#1F2937")
	tuiBgLight = lipgloss.Color("#374151")
)

var (
	styleLogo = lipgloss.NewStyle().
			Foreground(tuiAccent).
			Bold(true)

	styleTitle = lipgloss.NewStyle().
			Foreground(tuiText).
			Bold(true)

	styleSectionTitle = lipgloss.NewStyle().
				Foreground(tuiAccent2).
				Bold(true)

	styleActiveItem = lipgloss.NewStyle().
			Foreground(tuiText).
			Background(tuiAccent).
			Bold(true).
			Padding(0, 1)

	styleInactiveItem = lipgloss.NewStyle().
				Foreground(tuiMuted).
				Padding(0, 1)

	styleHint = lipgloss.NewStyle().
			Foreground(tuiFaint)

	styleSuccess = lipgloss.NewStyle().
			Foreground(tuiGreen)

	styleWarning = lipgloss.NewStyle().
			Foreground(tuiAmber)

	styleError = lipgloss.NewStyle().
			Foreground(tuiRed).
			Bold(true)

	styleLabel = lipgloss.NewStyle().
			Foreground(tuiText).
			Bold(true)

	styleValue = lipgloss.NewStyle().
			Foreground(tuiAccent2)

	styleDimValue = lipgloss.NewStyle().
			Foreground(tuiFaint)

	styleCard = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tuiBgLight).
			Padding(1, 2)

	styleCardTitle = lipgloss.NewStyle().
			Foreground(tuiAccent).
			Bold(true)

	styleToggleOn = lipgloss.NewStyle().
			Foreground(tuiGreen).
			Bold(true)

	styleToggleOff = lipgloss.NewStyle().
			Foreground(tuiFaint)

	styleStatusOn = lipgloss.NewStyle().
			Foreground(tuiGreen).
			Bold(true)

	styleStatusOff = lipgloss.NewStyle().
			Foreground(tuiFaint)

	styleProgressActive = lipgloss.NewStyle().
				Foreground(tuiAccent).
				Bold(true)

	styleProgressDone = lipgloss.NewStyle().
				Foreground(tuiGreen)

	styleProgressPending = lipgloss.NewStyle().
				Foreground(tuiFaint)

	styleReviewKey = lipgloss.NewStyle().
			Foreground(tuiMuted).
			Width(16)

	styleReviewValue = lipgloss.NewStyle().
				Foreground(tuiText)

	styleBreadcrumb = lipgloss.NewStyle().
			Foreground(tuiFaint)

	styleBreadcrumbActive = lipgloss.NewStyle().
				Foreground(tuiAccent2).
				Bold(true)

	styleInputPrompt = lipgloss.NewStyle().
				Foreground(tuiAccent2)

	styleConfirmed = lipgloss.NewStyle().
			Foreground(tuiMuted)
)
