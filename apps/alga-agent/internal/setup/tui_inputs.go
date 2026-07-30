package setup

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type listModel struct {
	choices []string
	cursor  int
	offset  int
	height  int
	width   int
}

func newList(choices []string, defIdx, width int) listModel {
	if defIdx < 0 || defIdx >= len(choices) {
		defIdx = 0
	}
	h := 8
	if len(choices) < h {
		h = len(choices)
	}
	m := listModel{
		choices: choices,
		cursor:  defIdx,
		height:  h,
		width:   width,
	}
	m.ensureVisible()
	return m
}

func (m *listModel) ensureVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
}

func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case "home", "g":
			m.cursor = 0
			m.ensureVisible()
		case "end", "G":
			m.cursor = len(m.choices) - 1
			m.ensureVisible()
		case "pgup":
			m.cursor = max(0, m.cursor-5)
			m.ensureVisible()
		case "pgdown":
			m.cursor = min(len(m.choices)-1, m.cursor+5)
			m.ensureVisible()
		}
	}
	return m, nil
}

func (m listModel) View() string {
	var b strings.Builder
	end := min(m.offset+m.height, len(m.choices))

	if m.offset > 0 {
		b.WriteString(styleHint.Render("    ···") + "\n")
	}
	for i := m.offset; i < end; i++ {
		label := m.choices[i]
		if m.width > 0 && len([]rune(label)) > m.width-6 {
			label = string([]rune(label)[:m.width-7]) + "…"
		}
		if i == m.cursor {
			b.WriteString("  " + styleActiveItem.Render(label) + "\n")
		} else {
			b.WriteString("  " + styleInactiveItem.Render(label) + "\n")
		}
	}
	if end < len(m.choices) {
		b.WriteString(styleHint.Render("    ···") + "\n")
	}
	return b.String()
}

type toggleModel struct {
	value bool
	label string
}

func newToggle(label string, def bool) toggleModel {
	return toggleModel{value: def, label: label}
}

func (m toggleModel) Update(msg tea.Msg) (toggleModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "left", "h", "y":
			m.value = true
		case "right", "l", "n":
			m.value = false
		case "tab":
			m.value = !m.value
		}
	}
	return m, nil
}

func (m toggleModel) View() string {
	var b strings.Builder
	if m.value {
		b.WriteString("  " + styleToggleOn.Render("● Yes") + "  " + styleToggleOff.Render("○ No"))
	} else {
		b.WriteString("  " + styleToggleOff.Render("○ Yes") + "  " + styleToggleOn.Render("● No"))
	}
	return b.String() + "\n"
}

type textModel struct {
	input  textinput.Model
	secret bool
}

func newText(def string, secret bool, width int) textModel {
	return newTextLimit(def, secret, width, 256)
}

func newTextLimit(def string, secret bool, width, charLimit int) textModel {
	ti := textinput.New()
	ti.SetWidth(width)
	ti.CharLimit = charLimit
	ti.SetValue(def)
	ti.Prompt = styleInputPrompt.Render("▸ ")
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	ti.Focus()
	return textModel{input: ti, secret: secret}
}

func (m textModel) Update(msg tea.Msg) (textModel, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m textModel) View() string {
	return "  " + m.input.View() + "\n"
}

func (m textModel) Value() string {
	return m.input.Value()
}
