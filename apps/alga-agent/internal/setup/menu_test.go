package setup

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeRawTerminal replays a scripted queue of keyEvents, implementing
// rawTerminal so driveMenu/driveYesNo can be exercised without a real TTY.
// restore records that it was called.
type fakeRawTerminal struct {
	keys          []keyEvent
	restoreCalled bool
}

func (f *fakeRawTerminal) readKey() (keyEvent, error) {
	if len(f.keys) == 0 {
		return keyEvent{}, errTerminalClosed
	}
	k := f.keys[0]
	f.keys = f.keys[1:]
	return k, nil
}

func (f *fakeRawTerminal) restore() error {
	f.restoreCalled = true
	return nil
}

// stripANSI removes SGR escape sequences so assertions can match visible text.
func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			in = true
		case in && (r == 'm' || r == 'K' || r == 'A'):
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TestDriveMenu_ArrowSelectFirst drives the cursor to the first item and
// confirms, asserting the returned index and that the final visible line is a
// clean confirmation (menu lines were cleared).
func TestDriveMenu_ArrowSelectFirst(t *testing.T) {
	choices := []string{"openrouter", "openai", "custom"}
	ft := &fakeRawTerminal{keys: []keyEvent{
		{kind: keyUp},    // already at 0 → stays
		{kind: keyEnter}, // confirm openrouter
	}}
	var out bytes.Buffer
	idx, err := driveMenu(ft, &out, "Provider", choices, 0)
	if err != nil {
		t.Fatalf("driveMenu: %v", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0", idx)
	}
	visible := stripANSI(out.String())
	if !strings.Contains(visible, "✓ Provider  openrouter") {
		t.Errorf("expected confirmation line containing '✓ Provider  openrouter', got:\n%s", visible)
	}
	// The intermediate menu frame (title + choices + hint) must have been
	// cleared via cursor-up sequences before the confirmation was printed.
	if !strings.Contains(out.String(), "\x1b[A") {
		t.Error("expected cursor-up clear sequence in output")
	}
}

func TestDriveMenu_ArrowDownThenConfirm(t *testing.T) {
	choices := []string{"openrouter", "openai", "custom"}
	ft := &fakeRawTerminal{keys: []keyEvent{
		{kind: keyDown},
		{kind: keyDown},
		{kind: keyEnter},
	}}
	var out bytes.Buffer
	idx, err := driveMenu(ft, &out, "Provider", choices, 0)
	if err != nil {
		t.Fatalf("driveMenu: %v", err)
	}
	if idx != 2 {
		t.Errorf("idx = %d, want 2 (custom)", idx)
	}
	visible := stripANSI(out.String())
	if !strings.Contains(visible, "✓ Provider  custom") {
		t.Errorf("expected '✓ Provider  custom', got:\n%s", visible)
	}
}

func TestDriveMenu_VimKeys(t *testing.T) {
	choices := []string{"openrouter", "openai", "custom"}
	ft := &fakeRawTerminal{keys: []keyEvent{
		{kind: keyRune, r: 'j'}, // down
		{kind: keyRune, r: 'j'}, // down → custom
		{kind: keyRune, r: 'k'}, // up → openai
		{kind: keyEnter},
	}}
	var out bytes.Buffer
	idx, err := driveMenu(ft, &out, "", choices, 0)
	if err != nil {
		t.Fatalf("driveMenu: %v", err)
	}
	if idx != 1 {
		t.Errorf("idx = %d, want 1 (openai)", idx)
	}
	visible := stripANSI(out.String())
	if !strings.Contains(visible, "✓ openai") {
		t.Errorf("expected '✓ openai', got:\n%s", visible)
	}
}

func TestDriveMenu_QuitRuneAborts(t *testing.T) {
	ft := &fakeRawTerminal{keys: []keyEvent{{kind: keyRune, r: 'q'}}}
	var out bytes.Buffer
	_, err := driveMenu(ft, &out, "", []string{"a", "b"}, 0)
	if !errors.Is(err, ErrAbort) {
		t.Errorf("err = %v, want ErrAbort", err)
	}
	if !strings.Contains(stripANSI(out.String()), "cancelled") {
		t.Errorf("expected cancelled line, got:\n%s", out.String())
	}
}

func TestDriveMenu_EscAborts(t *testing.T) {
	ft := &fakeRawTerminal{keys: []keyEvent{{kind: keyEsc}}}
	var out bytes.Buffer
	_, err := driveMenu(ft, &out, "Menu", []string{"a", "b"}, 1)
	if !errors.Is(err, ErrAbort) {
		t.Errorf("err = %v, want ErrAbort", err)
	}
}

func TestDriveMenu_HomeEnd(t *testing.T) {
	ft := &fakeRawTerminal{keys: []keyEvent{
		{kind: keyEnd},  // last
		{kind: keyHome}, // first
		{kind: keyEnter},
	}}
	var out bytes.Buffer
	idx, err := driveMenu(ft, &out, "", []string{"a", "b", "c"}, 0)
	if err != nil {
		t.Fatalf("driveMenu: %v", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0", idx)
	}
}

func TestDriveMenu_DefaultClamped(t *testing.T) {
	// defIdx out of range is clamped to 0; Enter confirms it.
	ft := &fakeRawTerminal{keys: []keyEvent{{kind: keyEnter}}}
	var out bytes.Buffer
	idx, err := driveMenu(ft, &out, "", []string{"a", "b"}, 99)
	if err != nil {
		t.Fatalf("driveMenu: %v", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0", idx)
	}
}

func TestDriveMenu_EmptyChoices(t *testing.T) {
	ft := &fakeRawTerminal{}
	var out bytes.Buffer
	_, err := driveMenu(ft, &out, "", nil, 0)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

// --- yes/no toggle ------------------------------------------------------

func TestDriveYesNo_DefaultTrue(t *testing.T) {
	ft := &fakeRawTerminal{keys: []keyEvent{{kind: keyEnter}}}
	var out bytes.Buffer
	got, err := driveYesNo(ft, &out, "Enable Telegram?", true)
	if err != nil {
		t.Fatalf("driveYesNo: %v", err)
	}
	if !got {
		t.Error("got false, want true")
	}
	visible := stripANSI(out.String())
	if !strings.Contains(visible, "✓ Enable Telegram  Yes") {
		t.Errorf("expected '✓ Enable Telegram  Yes', got:\n%s", visible)
	}
}

func TestDriveYesNo_Toggle(t *testing.T) {
	// Start true; one Right toggles to false; confirm.
	ft := &fakeRawTerminal{keys: []keyEvent{{kind: keyRight}, {kind: keyEnter}}}
	var out bytes.Buffer
	got, err := driveYesNo(ft, &out, "Enable Alga?", true)
	if err != nil {
		t.Fatalf("driveYesNo: %v", err)
	}
	if got {
		t.Error("got true, want false after toggle")
	}
	visible := stripANSI(out.String())
	if !strings.Contains(visible, "✓ Enable Alga  No") {
		t.Errorf("expected '✓ Enable Alga  No', got:\n%s", visible)
	}
}

func TestDriveYesNo_DirectLetters(t *testing.T) {
	// 'n' sets false directly even from a true default.
	ft := &fakeRawTerminal{keys: []keyEvent{{kind: keyRune, r: 'n'}, {kind: keyEnter}}}
	var out bytes.Buffer
	got, err := driveYesNo(ft, &out, "Respond in groups?", true)
	if err != nil {
		t.Fatalf("driveYesNo: %v", err)
	}
	if got {
		t.Error("got true, want false after 'n'")
	}
}

func TestDriveYesNo_EscAborts(t *testing.T) {
	ft := &fakeRawTerminal{keys: []keyEvent{{kind: keyEsc}}}
	var out bytes.Buffer
	_, err := driveYesNo(ft, &out, "Start fresh?", false)
	if !errors.Is(err, ErrAbort) {
		t.Errorf("err = %v, want ErrAbort", err)
	}
}

// TestMenuMetrics_ClearIsNoOpOnFirstRender guards the cursor-up logic: the very
// first render must not emit any cursor-up escapes because nothing was drawn
// yet.
func TestMenuMetrics_ClearIsNoOpOnFirstRender(t *testing.T) {
	var out bytes.Buffer
	m := menuMetrics{}
	m.render(&out, "T", []string{"a"}, 0)
	rendered := out.String()
	// After the first render, clear is invoked by the next render. Verify a
	// fresh metrics emits zero clear bytes.
	fresh := menuMetrics{}
	var clearBuf bytes.Buffer
	fresh.clear(&clearBuf)
	if clearBuf.Len() != 0 {
		t.Errorf("fresh clear wrote %d bytes, want 0", clearBuf.Len())
	}
	_ = rendered
}
