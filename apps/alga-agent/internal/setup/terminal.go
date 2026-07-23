package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/term"
)

// terminal.go provides the raw-terminal layer for arrow-key menu navigation.
//
// The interactive wizard uses a small, self-contained TUI on a TTY: it switches
// stdin to raw mode, decodes keypresses (including escape sequences for arrow
// keys), and lets menu.go render an in-place selectable list. When stdin is not
// a TTY (tests, CI, piped input, screen readers) the wizard falls back to the
// numbered/text prompts in setup.go, so nothing here is reachable in that mode.
//
// The decode logic (decodeKey) is kept pure and table-testable: it reads from
// any *bufio.Reader. The concrete *osTerminal wires that to os.Stdin and
// term.MakeRaw; tests use the fakeRawTerminal in menu_test.go instead.

// keyKind classifies a decoded keypress.
type keyKind int

const (
	keyRune keyKind = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyEnter
	keyEsc
	keyTab
	keyBackspace
	keyHome
	keyEnd
	keyPgUp
	keyPgDown
	keyDelete
	keyCtrlC
	keyCtrlD
	keyUnknown
)

// keyEvent is a decoded keypress. r is only meaningful for keyRune.
type keyEvent struct {
	kind keyKind
	r    rune
}

// rawTerminal is the minimal surface menu.go needs from the terminal. The real
// implementation is *osTerminal; tests inject fakeRawTerminal.
type rawTerminal interface {
	// readKey decodes and returns the next keypress. It blocks until input is
	// available. An unrecoverable read error or EOF is reported as err so the
	// caller can abort the wizard cleanly.
	readKey() (keyEvent, error)
	// restore returns the terminal to its prior (cooked) state. It must be safe
	// to call more than once.
	restore() error
}

// errTerminalClosed is returned by readKey when the underlying reader is
// exhausted (EOF) without an explicit Ctrl+C/D. Callers treat it as a cancel.
var errTerminalClosed = errors.New("terminal closed")

// openTerminal is the package-level seam used by promptChoice/promptYesNo to
// enter raw mode. It is a variable so tests can stub the TTY entry point; the
// real implementation is newOSTerminal. Returning an error here makes the
// caller fall back to the numbered/text prompts.
var openTerminal = func() (rawTerminal, error) { return newOSTerminal() }

// osTerminal wraps os.Stdin in raw mode and decodes keypresses from it.
type osTerminal struct {
	fd       int
	prev     *term.State // saved cooked state, restored on close
	r        *bufio.Reader
	restored bool
}

// newOSTerminal puts os.Stdin into raw mode and returns a terminal. It returns
// an error (rather than panicking) when raw mode is unavailable so the caller
// can degrade gracefully to text prompts.
func newOSTerminal() (*osTerminal, error) {
	fd := int(os.Stdin.Fd())
	prev, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("enter raw mode: %w", err)
	}
	return &osTerminal{
		fd:   fd,
		prev: prev,
		r:    bufio.NewReader(os.Stdin),
	}, nil
}

func (t *osTerminal) readKey() (keyEvent, error) {
	k, err := decodeKey(t.r)
	if err != nil {
		return keyEvent{}, err
	}
	return k, nil
}

func (t *osTerminal) restore() error {
	if t.restored {
		return nil
	}
	t.restored = true
	if t.prev == nil {
		return nil
	}
	if err := term.Restore(t.fd, t.prev); err != nil {
		return fmt.Errorf("restore terminal: %w", err)
	}
	return nil
}

// terminalWidth returns the current terminal width in cells, or 0 when it
// cannot be determined (non-TTY). A 0 result tells callers to skip width-based
// truncation.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 0
	}
	return w
}

// decodeKey reads the next keypress from r and classifies it. It understands:
//   - bare control bytes: \r/\n (enter), \t (tab), 0x7f/0x08 (backspace),
//     0x03 (Ctrl+C), 0x04 (Ctrl+D)
//   - CSI sequences: arrow keys (ESC [ A/B/C/D), Home/End (ESC [ H/F and the
//     1~ variant), PgUp/PgDn/Delete (ESC [ 5~/6~/3~)
//   - a lone ESC without a following byte (cancel)
//   - otherwise a UTF-8 rune
//
// Anything unrecognized after ESC returns keyUnknown and consumes the sequence
// so the next read starts cleanly.
func decodeKey(r *bufio.Reader) (keyEvent, error) {
	b, err := r.ReadByte()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return keyEvent{}, errTerminalClosed
		}
		return keyEvent{}, err
	}

	switch {
	case b == 0x1b: // ESC — start of a sequence or lone cancel
		return decodeEscape(r)
	case b == '\r' || b == '\n':
		return keyEvent{kind: keyEnter}, nil
	case b == '\t':
		return keyEvent{kind: keyTab}, nil
	case b == 0x7f || b == 0x08:
		return keyEvent{kind: keyBackspace}, nil
	case b == 0x03:
		return keyEvent{kind: keyCtrlC}, nil
	case b == 0x04:
		return keyEvent{kind: keyCtrlD}, nil
	case b < 0x20:
		// Other control bytes we don't model; treat as unknown.
		return keyEvent{kind: keyUnknown}, nil
	}

	// Multi-byte UTF-8 rune: reassemble with the leading byte. The size is
	// derived from the leading byte's high bits (standard UTF-8 prefixes),
	// not utf8.RuneLen — RuneLen takes a decoded rune value, not a byte.
	if b < 0x80 {
		return keyEvent{kind: keyRune, r: rune(b)}, nil
	}
	size := utf8SeqLen(b)
	if size <= 1 || size > 4 {
		// Invalid leading byte; consume defensively.
		return keyEvent{kind: keyUnknown}, nil
	}
	runeBytes := []byte{b}
	for i := 1; i < size; i++ {
		nb, err := r.ReadByte()
		if err != nil {
			return keyEvent{}, err
		}
		runeBytes = append(runeBytes, nb)
	}
	rr, _ := utf8.DecodeRune(runeBytes)
	if rr == utf8.RuneError {
		return keyEvent{kind: keyUnknown}, nil
	}
	return keyEvent{kind: keyRune, r: rr}, nil
}

// utf8SeqLen returns the number of bytes in a UTF-8 sequence given its leading
// byte, or 0 if b is not a valid leading byte.
func utf8SeqLen(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b&0xe0 == 0xc0:
		return 2
	case b&0xf0 == 0xe0:
		return 3
	case b&0xf8 == 0xf0:
		return 4
	default:
		return 0
	}
}

// decodeEscape handles the bytes following a leading ESC. peekByte lets us
// distinguish a lone ESC (cancel) from a CSI sequence without blocking forever:
// if no byte is buffered we treat it as cancel.
func decodeEscape(r *bufio.Reader) (keyEvent, error) {
	next, err := r.ReadByte()
	if err != nil {
		// No continuation: lone ESC = cancel. EOF still counts as cancel here.
		return keyEvent{kind: keyEsc}, nil
	}
	if next != '[' && next != 'O' {
		// ESC + a non-bracket byte (e.g. Meta-key). We don't model these; skip.
		return keyEvent{kind: keyUnknown}, nil
	}

	// Optional numeric parameter(s) between [ and the final letter/~, e.g.
	// ESC [ 5 ~ or ESC [ 3 ~. Collect leading digits.
	var param []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return keyEvent{kind: keyUnknown}, nil
		}
		if (b >= '0' && b <= '9') || b == ';' {
			param = append(param, b)
			continue
		}
		return csiFinal(b, param), nil
	}
}

// csiFinal maps the final byte of a CSI sequence (plus any numeric param) to a
// keyKind. Unknown combinations return keyUnknown.
func csiFinal(final byte, param []byte) keyEvent {
	switch final {
	case 'A':
		return keyEvent{kind: keyUp}
	case 'B':
		return keyEvent{kind: keyDown}
	case 'C':
		return keyEvent{kind: keyRight}
	case 'D':
		return keyEvent{kind: keyLeft}
	case 'H':
		return keyEvent{kind: keyHome}
	case 'F':
		return keyEvent{kind: keyEnd}
	case '~':
		switch string(param) {
		case "1", "7": // some terminals send Home as ESC[1~ or ESC[7~
			return keyEvent{kind: keyHome}
		case "4", "8": // End as ESC[4~ or ESC[8~
			return keyEvent{kind: keyEnd}
		case "5":
			return keyEvent{kind: keyPgUp}
		case "6":
			return keyEvent{kind: keyPgDown}
		case "3":
			return keyEvent{kind: keyDelete}
		}
	}
	return keyEvent{kind: keyUnknown}
}
