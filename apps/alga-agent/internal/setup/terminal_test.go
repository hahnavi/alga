package setup

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

// TestDecodeKey feeds scripted byte sequences through decodeKey and asserts the
// classified keyKind (and rune, for keyRune). This pins the escape-sequence
// handling that the arrow-key menus depend on.
func TestDecodeKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want keyEvent
	}{
		{"enter CR", "\r", keyEvent{kind: keyEnter}},
		{"enter LF", "\n", keyEvent{kind: keyEnter}},
		{"tab", "\t", keyEvent{kind: keyTab}},
		{"backspace DEL", "\x7f", keyEvent{kind: keyBackspace}},
		{"backspace BS", "\x08", keyEvent{kind: keyBackspace}},
		{"ctrl-c", "\x03", keyEvent{kind: keyCtrlC}},
		{"ctrl-d", "\x04", keyEvent{kind: keyCtrlD}},
		{"esc lone", "\x1b", keyEvent{kind: keyEsc}},
		{"arrow up", "\x1b[A", keyEvent{kind: keyUp}},
		{"arrow down", "\x1b[B", keyEvent{kind: keyDown}},
		{"arrow right", "\x1b[C", keyEvent{kind: keyRight}},
		{"arrow left", "\x1b[D", keyEvent{kind: keyLeft}},
		{"home CSI H", "\x1b[H", keyEvent{kind: keyHome}},
		{"end CSI F", "\x1b[F", keyEvent{kind: keyEnd}},
		{"home CSI 1~", "\x1b[1~", keyEvent{kind: keyHome}},
		{"end CSI 4~", "\x1b[4~", keyEvent{kind: keyEnd}},
		{"home CSI 7~", "\x1b[7~", keyEvent{kind: keyHome}},
		{"end CSI 8~", "\x1b[8~", keyEvent{kind: keyEnd}},
		{"pgup", "\x1b[5~", keyEvent{kind: keyPgUp}},
		{"pgdn", "\x1b[6~", keyEvent{kind: keyPgDown}},
		{"delete", "\x1b[3~", keyEvent{kind: keyDelete}},
		{"SS3 home (ESC O H)", "\x1bOH", keyEvent{kind: keyHome}},
		{"SS3 end (ESC O F)", "\x1bOF", keyEvent{kind: keyEnd}},
		{"rune a", "a", keyEvent{kind: keyRune, r: 'a'}},
		{"rune G", "G", keyEvent{kind: keyRune, r: 'G'}},
		{"rune ü (2-byte UTF-8)", "ü", keyEvent{kind: keyRune, r: 'ü'}},
		{"rune → (3-byte UTF-8)", "→", keyEvent{kind: keyRune, r: '→'}},
		{"unknown control NUL", "\x00", keyEvent{kind: keyUnknown}},
		{"esc + bare byte", "\x1bz", keyEvent{kind: keyUnknown}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader([]byte(tc.in)))
			got, err := decodeKey(r)
			if err != nil {
				t.Fatalf("decodeKey error: %v", err)
			}
			if got != tc.want {
				t.Errorf("decodeKey(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

// TestDecodeKey_EOF confirms an exhausted reader surfaces a sentinel error that
// the menu loop can treat as cancellation, never a bare nil-event.
func TestDecodeKey_EOF(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader(nil))
	_, err := decodeKey(r)
	if !errors.Is(err, errTerminalClosed) {
		t.Errorf("decodeKey on EOF err = %v, want errTerminalClosed", err)
	}
}

// TestDecodeKey_SequenceThenRune ensures multiple keys in the buffer decode
// independently in order.
func TestDecodeKey_SequenceThenRune(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("\x1b[B\x1b[Aj")))
	seq := []keyEvent{
		{kind: keyDown},
		{kind: keyUp},
		{kind: keyRune, r: 'j'},
	}
	for i, want := range seq {
		got, err := decodeKey(r)
		if err != nil {
			t.Fatalf("decodeKey #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("decodeKey #%d = %+v, want %+v", i, got, want)
		}
	}
	// Buffer now empty.
	if _, err := decodeKey(r); !errors.Is(err, errTerminalClosed) && !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF after draining, got %v", err)
	}
}
