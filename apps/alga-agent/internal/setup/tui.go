package setup

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"alga-agent/internal/config"
)

type wizardState int

const (
	stateMenu wizardState = iota
	stateSection
	stateReview
	stateDone
	stateQuit
)

type wizardModel struct {
	cfg        *config.Config
	state      wizardState
	sectionIdx int
	steps      []step
	stepIdx    int
	list       listModel
	toggle     toggleModel
	text       textModel
	confirmed  []string
	err        error
	width      int
	height     int
	menuCursor int
}

func newWizardModel(cfg *config.Config) wizardModel {
	return wizardModel{
		cfg:   cfg,
		state: stateMenu,
	}
}

func (m wizardModel) Init() tea.Cmd {
	return nil
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputWidth := min(60, max(30, m.width-12))
		m.text.input.Width = inputWidth
		m.list.width = inputWidth
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.state = stateQuit
			return m, tea.Quit
		}
	}

	switch m.state {
	case stateMenu:
		return m.updateMenu(msg)
	case stateSection:
		return m.updateSection(msg)
	case stateReview:
		return m.updateReview(msg)
	}
	return m, nil
}

func (m wizardModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.menuCursor > 0 {
				m.menuCursor--
			}
		case "down", "j":
			if m.menuCursor < len(sections)+1 {
				m.menuCursor++
			}
		case "enter":
			if m.menuCursor < len(sections) {
				m.enterSection(m.menuCursor)
			} else if m.menuCursor == len(sections) {
				m.state = stateReview
			} else {
				m.state = stateQuit
				return m, tea.Quit
			}
		case "q", "esc":
			m.state = stateQuit
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *wizardModel) enterSection(idx int) {
	m.sectionIdx = idx
	m.confirmed = nil
	s := sections[idx]
	switch s.key {
	case "model":
		m.steps = modelSteps(m.cfg)
	case "channel":
		m.steps = channelSteps(m.cfg)
	case "tools":
		m.steps = toolsSteps(m.cfg)
	case "behavior":
		m.steps = behaviorSteps(m.cfg)
	case "logging":
		m.steps = loggingSteps(m.cfg)
	}
	m.stepIdx = 0
	m.state = stateSection
	if len(m.steps) > 0 {
		m.initStep()
	}
}

func (m *wizardModel) advancePast(key string) {
	m.stepIdx = 0
	for i, s := range m.steps {
		if s.key == key {
			m.stepIdx = i + 1
			break
		}
	}
}

func (m *wizardModel) initStep() {
	s := m.steps[m.stepIdx]
	inputWidth := min(60, max(30, m.width-12))
	switch s.kind {
	case stepChoice:
		m.list = newList(s.choices, s.defIdx, inputWidth)
	case stepYesNo:
		m.toggle = newToggle(s.label, s.defBool)
	case stepText, stepSecret:
		if strings.Contains(s.key, "url") || strings.Contains(s.key, "webhook") {
			m.text = newTextLimit(s.def, s.kind == stepSecret, inputWidth, 2048)
		} else {
			m.text = newText(s.def, s.kind == stepSecret, inputWidth)
		}
	case stepInt:
		m.text = newText(strconv.Itoa(s.defInt), false, inputWidth)
	case stepFloat:
		m.text = newText(strconv.FormatFloat(s.defFloat, 'f', -1, 64), false, inputWidth)
	case stepDuration:
		m.text = newText(s.defDur.String(), false, inputWidth)
	case stepCSV:
		m.text = newTextLimit(strings.Join(s.defCSV, ", "), false, inputWidth, 2048)
	}
}

func (m wizardModel) updateSection(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.steps) == 0 {
		m.state = stateMenu
		return m, nil
	}
	s := m.steps[m.stepIdx]

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "esc":
			m.state = stateMenu
			return m, nil
		case "enter":
			return m.confirmStep()
		}
	}

	var cmd tea.Cmd
	switch s.kind {
	case stepChoice:
		m.list, cmd = m.list.Update(msg)
	case stepYesNo:
		m.toggle, cmd = m.toggle.Update(msg)
	default:
		m.text, cmd = m.text.Update(msg)
	}
	return m, cmd
}

func (m wizardModel) confirmStep() (tea.Model, tea.Cmd) {
	s := m.steps[m.stepIdx]
	var value string

	switch s.kind {
	case stepChoice:
		value = m.list.choices[m.list.cursor]
		if s.key == "channel_menu" {
			return m.handleChannelMenu()
		}
	case stepYesNo:
		value = strconv.FormatBool(m.toggle.value)
	default:
		value = m.text.Value()
	}

	if err := applyStepResult(m.cfg, s, value); err != nil {
		m.err = err
		return m, nil
	}
	m.err = nil
	m.confirmed = append(m.confirmed, styleSuccess.Render("✓")+" "+styleConfirmed.Render(s.label+": "+maskSecret(s, value)))

	if s.key == "provider" {
		m.steps = modelSteps(m.cfg)
		m.advancePast(s.key)
		if m.stepIdx >= len(m.steps) {
			m.state = stateMenu
			return m, nil
		}
		m.initStep()
		return m, nil
	}
	if s.key == "shell_enabled" || s.key == "search_enabled" || s.key == "search_provider" || s.key == "metrics_enabled" {
		m.steps = toolsSteps(m.cfg)
		if s.key == "metrics_enabled" {
			m.steps = loggingSteps(m.cfg)
		}
		m.advancePast(s.key)
		if m.stepIdx >= len(m.steps) {
			m.state = stateMenu
			return m, nil
		}
		m.initStep()
		return m, nil
	}
	if s.key == "tg_enabled" {
		m.steps = telegramSteps(m.cfg)
		m.advancePast(s.key)
		if m.stepIdx >= len(m.steps) {
			m.state = stateMenu
			return m, nil
		}
		m.initStep()
		return m, nil
	}
	if s.key == "alga_enabled" {
		m.steps = algaSteps(m.cfg)
		m.advancePast(s.key)
		if m.stepIdx >= len(m.steps) {
			m.state = stateMenu
			return m, nil
		}
		m.initStep()
		return m, nil
	}

	m.stepIdx++
	if m.stepIdx >= len(m.steps) {
		if sections[m.sectionIdx].key == "channel" && len(m.steps) > 1 {
			m.confirmed = nil
			m.steps = channelSteps(m.cfg)
			m.stepIdx = 0
			m.initStep()
			return m, nil
		}
		m.state = stateMenu
		return m, nil
	}
	m.initStep()
	return m, nil
}

func (m wizardModel) handleChannelMenu() (tea.Model, tea.Cmd) {
	idx := m.list.cursor
	switch idx {
	case 0:
		m.steps = telegramSteps(m.cfg)
		m.stepIdx = 0
		m.initStep()
	case 1:
		m.steps = algaSteps(m.cfg)
		m.stepIdx = 0
		m.initStep()
	case 2:
		m.state = stateMenu
	}
	return m, nil
}

func (m wizardModel) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "y", "enter":
			if verr := m.cfg.Validate(); verr != nil {
				m.err = verr
				return m, nil
			}
			m.err = nil
			m.state = stateDone
			return m, tea.Quit
		case "n", "esc", "q":
			m.state = stateMenu
		}
	}
	return m, nil
}

func (m wizardModel) View() string {
	switch m.state {
	case stateMenu:
		return m.viewMenu()
	case stateSection:
		return m.viewSection()
	case stateReview:
		return m.viewReview()
	case stateDone:
		return "\n  " + styleSuccess.Render("✓ Saving configuration…") + "\n\n"
	}
	return ""
}

const logo = `
   ╭───────────────────────────────────────╮
   │                                       │
   │     ◈  A L G A   A G E N T            │
   │        Configuration Wizard           │
   │                                       │
   ╰───────────────────────────────────────╯`

func (m wizardModel) viewMenu() string {
	var b strings.Builder

	b.WriteString(styleLogo.Render(logo) + "\n\n")

	for i, s := range sections {
		status := m.sectionStatus(s)
		if i == m.menuCursor {
			b.WriteString("  " + styleActiveItem.Render(" "+s.label+" ") + "  " + status + "\n")
		} else {
			b.WriteString("  " + styleInactiveItem.Render(" "+s.label+" ") + "  " + status + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styleHint.Render("  ─────────────────────────────────────") + "\n")
	if m.menuCursor == len(sections) {
		b.WriteString("  " + styleActiveItem.Render(" Review & Save ") + "\n")
	} else {
		b.WriteString("  " + styleInactiveItem.Render(" Review & Save ") + "\n")
	}
	if m.menuCursor == len(sections)+1 {
		b.WriteString("  " + styleActiveItem.Render(" Exit ") + "\n")
	} else {
		b.WriteString("  " + styleInactiveItem.Render(" Exit ") + "\n")
	}

	b.WriteString("\n  " + styleHint.Render("[↑/↓] navigate   [enter] select   [q] quit"))
	return b.String()
}

func (m wizardModel) sectionStatus(s sectionDef) string {
	status := s.status(m.cfg)
	if status == "" {
		return ""
	}
	return styleDimValue.Render(status)
}

func (m wizardModel) viewSection() string {
	var b strings.Builder
	s := sections[m.sectionIdx]

	b.WriteString("  " + styleBreadcrumb.Render("setup") +
		styleBreadcrumb.Render(" / ") +
		styleBreadcrumbActive.Render(s.label) + "\n\n")

	b.WriteString(m.progressDots() + "\n\n")

	for _, c := range m.confirmed {
		b.WriteString("  " + c + "\n")
	}
	if len(m.confirmed) > 0 {
		b.WriteString("\n")
	}

	if m.stepIdx < len(m.steps) {
		cur := m.steps[m.stepIdx]
		b.WriteString("  " + styleLabel.Render(cur.label) + "\n")
		if cur.help != "" {
			b.WriteString("  " + styleHint.Render(cur.help) + "\n")
		}
		if m.err != nil {
			b.WriteString("  " + styleError.Render("✗ "+m.err.Error()) + "\n")
		}
		b.WriteString("\n")

		switch cur.kind {
		case stepChoice:
			b.WriteString(m.list.View())
			b.WriteString("\n  " + styleHint.Render("[↑/↓] move   [enter] confirm   [esc] back"))
		case stepYesNo:
			b.WriteString(m.toggle.View())
			b.WriteString("\n  " + styleHint.Render("[←/→] toggle   [enter] confirm   [esc] back"))
		default:
			b.WriteString(m.text.View())
			b.WriteString("\n  " + styleHint.Render("[enter] confirm   [esc] back"))
		}
	}
	return b.String()
}

func (m wizardModel) progressDots() string {
	var b strings.Builder
	b.WriteString("  ")
	for i := range m.steps {
		switch {
		case i < m.stepIdx:
			b.WriteString(styleProgressDone.Render("● "))
		case i == m.stepIdx:
			b.WriteString(styleProgressActive.Render("● "))
		default:
			b.WriteString(styleProgressPending.Render("○ "))
		}
	}
	b.WriteString(styleDimValue.Render(fmt.Sprintf(" %d/%d", m.stepIdx+1, len(m.steps))))
	return b.String()
}

func (m wizardModel) viewReview() string {
	var b strings.Builder

	b.WriteString("  " + styleBreadcrumb.Render("setup") +
		styleBreadcrumb.Render(" / ") +
		styleBreadcrumbActive.Render("Review") + "\n\n")

	b.WriteString("  " + styleTitle.Render("Review Configuration") + "\n\n")
	b.WriteString(reviewView(m.cfg))
	b.WriteString("\n")

	if verr := m.cfg.Validate(); verr != nil {
		b.WriteString("  " + styleError.Render("✗ "+verr.Error()) + "\n\n")
	}

	b.WriteString("  " + styleLabel.Render("Save configuration?") + "  " +
		styleHint.Render("[y] save   [n] back"))
	return b.String()
}

func maskSecret(s step, value string) string {
	if s.kind == stepSecret && value != "" {
		return "••••••••"
	}
	if value == "" {
		return styleDimValue.Render("(empty)")
	}
	return value
}

func reviewView(cfg *config.Config) string {
	var b strings.Builder

	card := func(title string, rows [][2]string) {
		var inner strings.Builder
		for _, r := range rows {
			inner.WriteString(styleReviewKey.Render(r[0]) + styleReviewValue.Render(r[1]) + "\n")
		}
		b.WriteString("  " + styleCardTitle.Render("▎"+title) + "\n")
		b.WriteString("  " + strings.ReplaceAll(strings.TrimRight(inner.String(), "\n"), "\n", "\n  ") + "\n\n")
	}

	card("Model", [][2]string{
		{"Provider", cfg.Model.Provider},
		{"Base URL", cfg.Model.BaseURL},
		{"Model", cfg.Model.Model},
		{"Max tokens", strconv.Itoa(cfg.Model.MaxTokens)},
		{"Temperature", strconv.FormatFloat(cfg.Model.Temperature, 'f', -1, 64)},
		{"API key", secretBadge(cfg.Model.APIKey)},
	})

	card("Channels", [][2]string{
		{"Telegram", onOffBadge(cfg.Telegram.Enabled) + "  token: " + secretBadge(cfg.Telegram.BotToken)},
		{"Alga", onOffBadge(cfg.Alga.Enabled) + "  token: " + secretBadge(cfg.Alga.AgentToken)},
	})

	shell := "off"
	if cfg.Tools.Shell.Enabled {
		shell = fmt.Sprintf("on (%d commands)", len(cfg.Tools.Shell.AllowedCommands))
	}
	search := "off"
	if cfg.Tools.WebSearch.Enabled {
		search = "on (" + cfg.Tools.WebSearch.Provider + ")"
	}
	card("Tools", [][2]string{
		{"Shell", shell},
		{"Web search", search},
	})

	card("Behavior", [][2]string{
		{"Iterations", strconv.Itoa(cfg.AgentBehavior.MaxIterations)},
		{"Tool timeout", cfg.AgentBehavior.ToolTimeout.String()},
		{"Context window", strconv.Itoa(cfg.AgentBehavior.ContextWindow)},
	})

	metrics := "off"
	if cfg.Metrics.Enabled {
		metrics = "on (" + cfg.Metrics.Addr + ")"
	}
	card("Logging & Metrics", [][2]string{
		{"Level", cfg.Logging.Level},
		{"Metrics", metrics},
	})

	return b.String()
}

func secretBadge(v string) string {
	if v == "" {
		return styleError.Render("✗ not set")
	}
	return styleSuccess.Render("✓ set")
}

func onOffBadge(v bool) string {
	if v {
		return styleStatusOn.Render("on")
	}
	return styleStatusOff.Render("off")
}
