package setup

import (
	"fmt"
	"io"
	"strings"
)

// menu.go implements the arrow-key TUI for choice menus and yes/no toggles.
//
// driveMenu and driveYesNo are pure with respect to terminal I/O: they read
// keypresses from a rawTerminal and write ANSI to an io.Writer. The only
// terminal-state side effect (raw mode) lives in terminal.go's osTerminal, so
// these functions are unit-testable with a fake terminal that replays a
// scripted key sequence.
//
// Rendering is in-place: each render clears the previously drawn lines with
// ANSI cursor-up + clear-to-EOL and redraws. On confirm or cancel the whole
// block is cleared and replaced with a single confirmation line, so the screen
// stays clean as the wizard progresses.

// menuMetrics sizes the drawn frame so we can erase exactly the lines we wrote.
type menuMetrics struct {
	lines int // number of screen lines drawn by the last render
}

// driveMenu shows a selectable list and returns the chosen index, or ErrAbort
// if the user cancels with Esc / Ctrl+C / Ctrl+D. defIdx is the initial cursor
// position (clamped to the choice range); title is printed above the list when
// non-empty.
func driveMenu(rt rawTerminal, w io.Writer, title string, choices []string, defIdx int) (int, error) {
	if len(choices) == 0 {
		return 0, fmt.Errorf("no choices available")
	}
	sel := defIdx
	if sel < 0 || sel >= len(choices) {
		sel = 0
	}

	m := menuMetrics{}
	m.render(w, title, choices, sel)

	for {
		k, err := rt.readKey()
		if err != nil {
			m.clear(w)
			return 0, err
		}
		switch k.kind {
		case keyUp, keyLeft:
			if sel > 0 {
				sel--
			}
		case keyDown, keyRight:
			if sel < len(choices)-1 {
				sel++
			}
		case keyHome:
			sel = 0
		case keyEnd:
			sel = len(choices) - 1
		case keyPgUp:
			sel = clamp(sel-5, 0, len(choices)-1)
		case keyPgDown:
			sel = clamp(sel+5, 0, len(choices)-1)
		case keyEnter:
			m.clear(w)
			renderMenuConfirm(w, title, choices[sel])
			return sel, nil
		case keyEsc, keyCtrlC, keyCtrlD:
			m.clear(w)
			renderCancelled(w)
			return 0, ErrAbort
		case keyRune:
			switch k.r {
			case 'k': // vim-style up
				if sel > 0 {
					sel--
				}
			case 'j': // vim-style down
				if sel < len(choices)-1 {
					sel++
				}
			case 'g': // vim-style top
				sel = 0
			case 'G': // vim-style bottom
				sel = len(choices) - 1
			case 'q': // quit
				m.clear(w)
				renderCancelled(w)
				return 0, ErrAbort
			}
		}
		m.render(w, title, choices, sel)
	}
}

// driveYesNo shows a Yes/No toggle and returns the selection, or ErrAbort on
// cancel. def is the initial position.
func driveYesNo(rt rawTerminal, w io.Writer, question string, def bool) (bool, error) {
	val := def
	m := menuMetrics{}
	m.renderYesNo(w, question, val)

	for {
		k, err := rt.readKey()
		if err != nil {
			m.clear(w)
			return false, err
		}
		switch k.kind {
		case keyLeft, keyRight, keyTab:
			val = !val
		case keyHome:
			val = true
		case keyEnd:
			val = false
		case keyEnter:
			m.clear(w)
			renderYesNoConfirm(w, question, val)
			return val, nil
		case keyEsc, keyCtrlC, keyCtrlD:
			m.clear(w)
			renderCancelled(w)
			return false, ErrAbort
		case keyRune:
			switch k.r {
			case 'h': // vim-style left
				val = true
			case 'l': // vim-style right
				val = false
			case 'y', 'Y':
				val = true
			case 'n', 'N':
				val = false
			case 'q':
				m.clear(w)
				renderCancelled(w)
				return false, ErrAbort
			}
		}
		m.renderYesNo(w, question, val)
	}
}

// --- rendering -----------------------------------------------------------

// render draws the menu frame (title + choices + hint) and records how many
// lines were emitted so the next render or clear can erase exactly them.
//
// Layout: the active row is "❯ <label>" in cyan/bold; inactive rows are
// "  <label>" dim. Both share the same label column so labels line up as the
// cursor moves. Long labels are truncated with an ellipsis when the terminal
// width is known.
func (m *menuMetrics) render(w io.Writer, title string, choices []string, sel int) {
	m.clear(w)

	width := terminalWidth()
	var b strings.Builder
	if title != "" {
		b.WriteString(color(title, colorBold))
		b.WriteString("\r\n")
	}
	for i, c := range choices {
		label := truncateLabel(c, width)
		if i == sel {
			fmt.Fprintf(&b, " %s %s\r\n", color("❯", colorCyan+colorBold), color(label, colorCyan+colorBold))
			continue
		}
		fmt.Fprintf(&b, " %s %s\r\n", color(" ", colorDim), color(label, colorDim))
	}
	b.WriteString(color("  ↑↓ move · ⏎ confirm · esc cancel", colorDim))
	b.WriteString("\r\n")

	io.WriteString(w, b.String())
	m.lines = countLines(b.String())
}

// truncateLabel shortens s so the rendered row fits within width cells, leaving
// room for the "❯ " prefix. width <= 0 means unknown; return s unchanged.
func truncateLabel(s string, width int) string {
	if width <= 0 {
		return s
	}
	// Reserve 2 cells for "❯ " and a safety margin of 1.
	avail := width - 3
	if avail <= 0 {
		return s
	}
	if runeLen(s) <= avail {
		return s
	}
	return truncateRunes(s, max(1, avail-1)) + "…"
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]rune, 0, n)
	i := 0
	for _, r := range s {
		if i >= n {
			break
		}
		out = append(out, r)
		i++
	}
	return string(out)
}

func (m *menuMetrics) renderYesNo(w io.Writer, question string, val bool) {
	m.clear(w)

	var b strings.Builder
	b.WriteString(color(question, colorBold))
	b.WriteString("\r\n")
	if val {
		fmt.Fprintf(&b, "  %s   %s\r\n",
			color("● Yes", colorGreen+colorBold),
			color("○ No", colorDim))
	} else {
		fmt.Fprintf(&b, "  %s   %s\r\n",
			color("○ Yes", colorDim),
			color("● No", colorGreen+colorBold))
	}
	b.WriteString(color("  ←→ toggle · ⏎ confirm · esc cancel", colorDim))
	b.WriteString("\r\n")

	io.WriteString(w, b.String())
	m.lines = countLines(b.String())
}

// clear erases the previously drawn lines by moving the cursor up and clearing
// each line. It is a no-op when nothing has been drawn yet.
func (m *menuMetrics) clear(w io.Writer) {
	if m.lines == 0 {
		return
	}
	// Move up N lines, clearing each, then return to the start of the first.
	var b strings.Builder
	for i := 0; i < m.lines; i++ {
		b.WriteString("\033[A") // cursor up one line
		b.WriteString("\r")
		b.WriteString("\033[2K") // clear entire line
	}
	io.WriteString(w, b.String())
	m.lines = 0
}

// renderMenuConfirm prints a single confirmation line after a selection.
func renderMenuConfirm(w io.Writer, label, value string) {
	if label == "" {
		fmt.Fprintf(w, "%s %s\r\n", color("✓", colorGreen), color(value, colorBold))
		return
	}
	fmt.Fprintf(w, "%s %s  %s\r\n", color("✓", colorGreen), color(label, colorDim), color(value, colorBold))
}

func renderYesNoConfirm(w io.Writer, question string, val bool) {
	word := "Yes"
	if !val {
		word = "No"
	}
	// Trim the trailing hint-style punctuation from the question for a clean
	// confirmation line ("Enable Telegram?" -> "Enable Telegram  Yes").
	q := strings.TrimRight(strings.TrimSpace(question), "?")
	fmt.Fprintf(w, "%s %s  %s\r\n", color("✓", colorGreen), color(q, colorDim), color(word, colorBold))
}

func renderCancelled(w io.Writer) {
	fmt.Fprintf(w, "%s\r\n", color("cancelled", colorDim))
}

// countLines returns the number of \n-terminated lines in s. Each render ends
// its lines with "\r\n", so this counts the drawn rows.
func countLines(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			n++
		}
	}
	return n
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
