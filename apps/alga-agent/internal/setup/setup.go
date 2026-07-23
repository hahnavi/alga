// Package setup implements the interactive `alga-agent setup` wizard.
//
// On a TTY the wizard uses an arrow-key TUI: choice menus and yes/no prompts
// render as in-place selectable lists (see menu.go and terminal.go). On a
// non-TTY (tests, CI, piped input, screen readers) it falls back to numbered
// and y/n text prompts so the flow stays scriptable. Section-based dispatch
// lets a user run `alga-agent setup model` or `alga-agent setup channel` to
// jump straight to one part, or `alga-agent setup` for the full menu.
//
// The wizard loads the existing config (falling back to defaults when none
// exists), prompts with current values shown as defaults, and writes the result
// back to the resolved config path with mode 0600. Secrets (API keys, bot
// tokens) are stored literally in config.yaml, mirroring the hermes convention.
package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"alga-agent/internal/config"
)

// ANSI color codes. Output is plain when color is disabled.
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorMag    = "\033[35m"
)

func shouldUseColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func color(s, code string) string {
	if !shouldUseColor() {
		return s
	}
	return code + s + colorReset
}

// --- output helpers -------------------------------------------------------

func printHeader(w io.Writer, title string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, color("◆ "+title, colorCyan+colorBold))
}

func printInfo(w io.Writer, text string) {
	fmt.Fprintln(w, color("  "+text, colorDim))
}

func printSuccess(w io.Writer, text string) {
	fmt.Fprintln(w, color("✓ "+text, colorGreen))
}

func printWarning(w io.Writer, text string) {
	fmt.Fprintln(w, color("⚠ "+text, colorYellow))
}

func printError(w io.Writer, text string) {
	fmt.Fprintln(w, color("✗ "+text, colorRed))
}

// --- input helpers --------------------------------------------------------
//
// All helpers take a *bufio.Reader so the flow is fully testable: tests feed a
// scripted input buffer and assert on the resulting config mutation.

// ErrAbort signals an intentional user cancellation (Ctrl+C / EOF). Callers
// may treat it as a non-fatal exit.
var ErrAbort = errors.New("setup interrupted")

// prompt reads a line, returning def when the user presses Enter. A leading
// Ctrl+D (EOF) or Ctrl+C aborts the wizard.
func prompt(r *bufio.Reader, w io.Writer, question, def string) (string, error) {
	display := question
	if def != "" {
		display += " [" + def + "]"
	}
	display += ": "
	fmt.Fprint(w, color(display, colorYellow))
	line, err := r.ReadString('\n')
	if err != nil && (err == io.EOF && line == "") {
		return "", ErrAbort
	}
	v := strings.TrimSpace(line)
	if v == "" {
		return def, nil
	}
	return v, nil
}

// promptSecret behaves like prompt but echoes '*' per typed character. It only
// works on a real TTY; on a non-TTY (tests, piped input) it falls back to a
// plain prompt so the flow remains drivable.
func promptSecret(r *bufio.Reader, w io.Writer, question, def string) (string, error) {
	if !stdinIsTerminal() {
		// Non-interactive: keep the default without revealing it, unless the
		// caller passed an empty default (first-time setup).
		if def == "" {
			return prompt(r, w, question, "")
		}
		fmt.Fprintln(w, color("  "+question+" [set] (press Enter to keep): ", colorYellow))
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return "", ErrAbort
		}
		if v := strings.TrimSpace(line); v != "" {
			return v, nil
		}
		return def, nil
	}
	// TTY path: read byte-by-byte, masking with '*'.
	fmt.Fprint(w, color(question+" (input hidden)", colorYellow))
	if def != "" {
		fmt.Fprint(w, color(" [set, Enter to keep]", colorYellow))
	}
	fmt.Fprint(w, color(": ", colorYellow))
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(w)
				if len(buf) == 0 {
					if def != "" {
						return def, nil
					}
					return "", ErrAbort
				}
				return string(buf), nil
			}
			return "", ErrAbort
		}
		switch b {
		case '\n', '\r':
			fmt.Fprintln(w)
			if len(buf) == 0 {
				return def, nil
			}
			return string(buf), nil
		case 3, 4: // Ctrl+C, Ctrl+D
			fmt.Fprintln(w)
			if len(buf) == 0 {
				if def != "" {
					return def, nil
				}
				return "", ErrAbort
			}
			return string(buf), nil
		case 127, 8: // backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(w, "\b \b")
			}
		default:
			buf = append(buf, b)
			fmt.Fprint(w, "*")
		}
	}
}

// promptChoice shows a selection menu. On a TTY it uses an arrow-key list
// (menu.go); otherwise it falls back to a numbered text menu. An empty input in
// text mode selects defIdx.
func promptChoice(r *bufio.Reader, w io.Writer, question string, choices []string, defIdx int) (int, error) {
	if stdinIsTerminal() {
		if rt, err := openTerminal(); err == nil {
			defer rt.restore()
			return driveMenu(rt, w, question, choices, defIdx)
		}
	}
	return promptChoiceText(r, w, question, choices, defIdx)
}

// promptChoiceText prints a numbered menu and returns the selected index. An
// empty input selects defIdx. Used on non-TTY stdin (tests, piped input).
func promptChoiceText(r *bufio.Reader, w io.Writer, question string, choices []string, defIdx int) (int, error) {
	fmt.Fprintln(w, color(question, colorBold))
	for i, c := range choices {
		if i == defIdx {
			fmt.Fprintln(w, color(fmt.Sprintf("  %d) %s", i+1, c), colorGreen))
		} else {
			fmt.Fprintf(w, "  %d) %s\n", i+1, c)
		}
	}
	for {
		fmt.Fprint(w, color(fmt.Sprintf("  Select [1-%d] (%d): ", len(choices), defIdx+1), colorDim))
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return 0, ErrAbort
		}
		v := strings.TrimSpace(line)
		if v == "" {
			return defIdx, nil
		}
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 || n > len(choices) {
			printError(w, fmt.Sprintf("Please enter a number between 1 and %d", len(choices)))
			continue
		}
		return n - 1, nil
	}
}

// promptYesNo asks a yes/no question. On a TTY it renders a Yes/No toggle
// driven by arrow keys (menu.go); otherwise it falls back to y/n text input.
func promptYesNo(r *bufio.Reader, w io.Writer, question string, def bool) (bool, error) {
	if stdinIsTerminal() {
		if rt, err := openTerminal(); err == nil {
			defer rt.restore()
			return driveYesNo(rt, w, question, def)
		}
	}
	return promptYesNoText(r, w, question, def)
}

// promptYesNoText asks a y/n question via text input. Used on non-TTY stdin.
func promptYesNoText(r *bufio.Reader, w io.Writer, question string, def bool) (bool, error) {
	hint := "Y/n"
	if !def {
		hint = "y/N"
	}
	for {
		fmt.Fprint(w, color(fmt.Sprintf("%s [%s]: ", question, hint), colorYellow))
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return false, ErrAbort
		}
		v := strings.ToLower(strings.TrimSpace(line))
		switch v {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		printError(w, "Please enter 'y' or 'n'")
	}
}

// promptInt asks for an integer. Empty input or a parse error keeps def (with a
// warning printed for genuinely invalid input so the user knows it was ignored).
func promptInt(r *bufio.Reader, w io.Writer, question string, def int) (int, error) {
	s, err := prompt(r, w, question, strconv.Itoa(def))
	if err != nil {
		return 0, err
	}
	n, perr := strconv.Atoi(s)
	if perr != nil {
		printWarning(w, "Invalid number, keeping "+strconv.Itoa(def))
		return def, nil
	}
	return n, nil
}

// promptFloat asks for a float64, keeping def on empty or invalid input.
func promptFloat(r *bufio.Reader, w io.Writer, question string, def float64) (float64, error) {
	s, err := prompt(r, w, question, strconv.FormatFloat(def, 'f', -1, 64))
	if err != nil {
		return 0, err
	}
	n, perr := strconv.ParseFloat(s, 64)
	if perr != nil {
		printWarning(w, "Invalid number, keeping previous value")
		return def, nil
	}
	return n, nil
}

// promptDuration asks for a Go duration string (e.g. "30s", "5m"), keeping def
// on empty or invalid input.
func promptDuration(r *bufio.Reader, w io.Writer, question string, def time.Duration) (time.Duration, error) {
	s, err := prompt(r, w, question, def.String())
	if err != nil {
		return 0, err
	}
	d, perr := config.ParseDuration(s)
	if perr != nil {
		printWarning(w, "Invalid duration, keeping "+def.String())
		return def, nil
	}
	return d, nil
}

// promptCSV asks for a comma-separated list, returning a cleaned slice. Empty
// entries are dropped. An empty input returns def unchanged.
func promptCSV(r *bufio.Reader, w io.Writer, question string, def []string) ([]string, error) {
	defStr := strings.Join(def, ", ")
	s, err := prompt(r, w, question, defStr)
	if err != nil {
		return nil, err
	}
	if s == defStr {
		return def, nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// isInteractive reports whether the wizard can run. It is false when stdin is
// not a TTY (headless SSH, CI, piped input) or when ALGA_AGENT_NONINTERACTIVE
// is a truthy value.
func isInteractive() bool {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("ALGA_AGENT_NONINTERACTIVE"))); v != "" {
		switch v {
		case "1", "true", "yes", "on", "y", "t":
			return false
		}
	}
	return stdinIsTerminal()
}

// --- section flows ---------------------------------------------------------

func baseURLForProvider(provider string) string {
	switch provider {
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

// setupModel prompts for the LLM provider and model settings, mutating cfg in
// place. Current values are shown as defaults so re-running is non-destructive.
func setupModel(cfg *config.Config, r *bufio.Reader, w io.Writer) error {
	printHeader(w, "Model & Provider")
	printInfo(w, "Choose how to connect to your chat model (any OpenAI-compatible endpoint).")

	providers := []string{"openai", "openrouter", "custom"}
	defIdx := 0
	for i, p := range providers {
		if p == cfg.Model.Provider {
			defIdx = i
		}
	}
	idx, err := promptChoice(r, w, "Provider:", providers, defIdx)
	if err != nil {
		return err
	}
	cfg.Model.Provider = providers[idx]

	// For known providers suggest the canonical URL as the default (so switching
	// from openai to openrouter updates the suggestion, rather than clinging to
	// the stale openai URL). Custom has no canonical default, so start from
	// whatever is already set.
	defBase := cfg.Model.BaseURL
	if cfg.Model.Provider != "custom" {
		defBase = baseURLForProvider(cfg.Model.Provider)
	} else if defBase == "" {
		defBase = baseURLForProvider("openai")
	}
	base, err := prompt(r, w, "Base URL", defBase)
	if err != nil {
		return err
	}
	cfg.Model.BaseURL = base

	key, err := promptSecret(r, w, "API key", cfg.Model.APIKey)
	if err != nil {
		return err
	}
	cfg.Model.APIKey = key

	model, err := prompt(r, w, "Model", orDefault(cfg.Model.Model, "gpt-4o"))
	if err != nil {
		return err
	}
	cfg.Model.Model = model

	cfg.Model.MaxTokens, err = promptInt(r, w, "Max tokens", orNonZero(cfg.Model.MaxTokens, 4096))
	if err != nil {
		return err
	}
	cfg.Model.Temperature, err = promptFloat(r, w, "Temperature", orNonZeroF(cfg.Model.Temperature, 0.3))
	if err != nil {
		return err
	}
	return nil
}

// channelContinueIdx is the index of the "Continue" item in the channel menu.
const channelContinueIdx = 2

// channelMenuChoices builds the channel list shown in the interactive menu. The
// Telegram and Alga rows display their current on/off state so the user can see
// what is configured at a glance. The final row is the "Continue" action that
// finishes channel setup and returns to the parent menu.
func channelMenuChoices(cfg *config.Config) []string {
	return []string{
		channelRowLabel("Telegram", cfg.Telegram.Enabled, "(human interface)"),
		channelRowLabel("Alga", cfg.Alga.Enabled, "(investigation threads)"),
		"Continue",
	}
}

// channelRowLabel renders a channel list row with its live on/off status.
func channelRowLabel(name string, enabled bool, hint string) string {
	status := color("off", colorDim)
	if enabled {
		status = color("on", colorGreen)
	}
	if hint != "" {
		return fmt.Sprintf("%s %s  %s", name, status, color(hint, colorDim))
	}
	return fmt.Sprintf("%s %s", name, status)
}

// setupChannels presents an interactive channel list: each channel can be
// opened for configuration, and changes are reflected live in the list. The
// "Continue" row finishes setup. The at-least-one-channel invariant is not
// enforced here — it is checked at the Review & Save step via Validate(), so a
// user can leave channels disabled and fix it (or enable Alga instead) later.
func setupChannels(cfg *config.Config, r *bufio.Reader, w io.Writer) error {
	printHeader(w, "Channels")
	printInfo(w, "Select a channel to configure, or Continue when done.")
	printInfo(w, "At least one channel must be enabled before you can save.")

	// Default cursor to the first channel so a first-time user lands on it.
	defIdx := 0
	for {
		idx, err := promptChoice(r, w, "", channelMenuChoices(cfg), defIdx)
		if err != nil {
			return err
		}
		switch idx {
		case 0:
			if err := setupTelegram(cfg, r, w); err != nil {
				return err
			}
		case 1:
			if err := setupAlga(cfg, r, w); err != nil {
				return err
			}
		case channelContinueIdx:
			return nil
		}
		// Keep the cursor on the channel just edited for quick re-edits.
		defIdx = idx
	}
}

// setupTelegram prompts for the Telegram channel settings and mutates cfg in
// place. Current values are shown as defaults so re-running is non-destructive.
func setupTelegram(cfg *config.Config, r *bufio.Reader, w io.Writer) error {
	printHeader(w, "Telegram (human interface)")
	tgEnabled, err := promptYesNo(r, w, "Enable Telegram channel?", cfg.Telegram.Enabled)
	if err != nil {
		return err
	}
	cfg.Telegram.Enabled = tgEnabled
	if !tgEnabled {
		return nil
	}

	token, err := promptSecret(r, w, "Bot token (from @BotFather)", cfg.Telegram.BotToken)
	if err != nil {
		return err
	}
	cfg.Telegram.BotToken = token

	webhook, err := prompt(r, w, "Webhook URL (empty = long polling)", cfg.Telegram.WebhookURL)
	if err != nil {
		return err
	}
	cfg.Telegram.WebhookURL = webhook

	addr, err := prompt(r, w, "Webhook listen address", orDefault(cfg.Telegram.WebhookAddr, "0.0.0.0:8443"))
	if err != nil {
		return err
	}
	cfg.Telegram.WebhookAddr = addr

	respondInGroups, err := promptYesNo(r, w, "Respond in groups when not @mentioned?", cfg.Telegram.RespondInGroups)
	if err != nil {
		return err
	}
	cfg.Telegram.RespondInGroups = respondInGroups
	return nil
}

// setupAlga prompts for the Alga channel settings and mutates cfg in place.
func setupAlga(cfg *config.Config, r *bufio.Reader, w io.Writer) error {
	printHeader(w, "Alga (investigation threads)")
	algaEnabled, err := promptYesNo(r, w, "Enable Alga channel?", cfg.Alga.Enabled)
	if err != nil {
		return err
	}
	cfg.Alga.Enabled = algaEnabled
	if !algaEnabled {
		return nil
	}

	server, err := prompt(r, w, "Alga server URL", orDefault(cfg.Alga.ServerURL, "http://localhost:8080"))
	if err != nil {
		return err
	}
	cfg.Alga.ServerURL = server

	token, err := promptSecret(r, w, "Agent token", cfg.Alga.AgentToken)
	if err != nil {
		return err
	}
	cfg.Alga.AgentToken = token
	return nil
}

// setupTools prompts for the shell and web-search tool settings. The shell
// allowlist is security-relevant: commands run with the agent's privileges, so
// the wizard reminds the user this is not a sandbox.
func setupTools(cfg *config.Config, r *bufio.Reader, w io.Writer) error {
	printHeader(w, "Tools")
	printInfo(w, "Shell runs commands with the agent's privileges — restrict the allowlist.")

	// --- Shell ---
	shellOn, err := promptYesNo(r, w, "Enable shell tool?", cfg.Tools.Shell.Enabled)
	if err != nil {
		return err
	}
	cfg.Tools.Shell.Enabled = shellOn
	if shellOn {
		allowed, err := promptCSV(r, w, "Allowed commands (comma-separated)", cfg.Tools.Shell.AllowedCommands)
		if err != nil {
			return err
		}
		cfg.Tools.Shell.AllowedCommands = allowed

		cfg.Tools.Shell.MaxOutputBytes, err = promptInt(r, w, "Max output bytes", orNonZero(cfg.Tools.Shell.MaxOutputBytes, 10240))
		if err != nil {
			return err
		}
		cfg.Tools.Shell.Timeout, err = promptDuration(r, w, "Timeout", orNonZeroDur(cfg.Tools.Shell.Timeout, 30*time.Second))
		if err != nil {
			return err
		}
	}

	// --- Web search ---
	printInfo(w, "Web search lets the agent look up current information.")
	searchOn, err := promptYesNo(r, w, "Enable web search?", cfg.Tools.WebSearch.Enabled)
	if err != nil {
		return err
	}
	cfg.Tools.WebSearch.Enabled = searchOn
	if !searchOn {
		return nil
	}

	providers := []string{"duckduckgo", "brave", "tavily"}
	defIdx := 0
	for i, p := range providers {
		if p == cfg.Tools.WebSearch.Provider {
			defIdx = i
		}
	}
	idx, err := promptChoice(r, w, "Search provider:", providers, defIdx)
	if err != nil {
		return err
	}
	cfg.Tools.WebSearch.Provider = providers[idx]

	// brave/tavily require an API key; duckduckgo does not.
	if cfg.Tools.WebSearch.Provider != "duckduckgo" {
		key, err := promptSecret(r, w, "Search API key", cfg.Tools.WebSearch.APIKey)
		if err != nil {
			return err
		}
		cfg.Tools.WebSearch.APIKey = key
	}

	cfg.Tools.WebSearch.MaxResults, err = promptInt(r, w, "Max results", orNonZero(cfg.Tools.WebSearch.MaxResults, 5))
	if err != nil {
		return err
	}

	fetch, err := promptYesNo(r, w, "Fetch full page content for results?", cfg.Tools.WebSearch.FetchContent)
	if err != nil {
		return err
	}
	cfg.Tools.WebSearch.FetchContent = fetch
	return nil
}

// setupBehavior prompts for agent behavior tuning: iteration cap, tool timeout,
// context window, and an optional custom system prompt file path.
func setupBehavior(cfg *config.Config, r *bufio.Reader, w io.Writer) error {
	printHeader(w, "Agent Behavior")
	printInfo(w, "Tune the conversation loop. Defaults are sensible for most setups.")

	var err error
	cfg.AgentBehavior.MaxIterations, err = promptInt(r, w, "Max iterations per message", orNonZero(cfg.AgentBehavior.MaxIterations, 30))
	if err != nil {
		return err
	}
	cfg.AgentBehavior.ToolTimeout, err = promptDuration(r, w, "Per-tool timeout", orNonZeroDur(cfg.AgentBehavior.ToolTimeout, 30*time.Second))
	if err != nil {
		return err
	}
	cfg.AgentBehavior.ContextWindow, err = promptInt(r, w, "Context window (messages retained)", orNonZero(cfg.AgentBehavior.ContextWindow, 20))
	if err != nil {
		return err
	}
	promptFile, err := prompt(r, w, "System prompt file (empty = built-in)", cfg.AgentBehavior.SystemPromptFile)
	if err != nil {
		return err
	}
	cfg.AgentBehavior.SystemPromptFile = promptFile
	return nil
}

// setupLogging prompts for log level/destination and the optional Prometheus
// metrics endpoint.
func setupLogging(cfg *config.Config, r *bufio.Reader, w io.Writer) error {
	printHeader(w, "Logging & Metrics")

	levels := []string{"debug", "info", "warn", "error"}
	defIdx := 0
	for i, l := range levels {
		if l == cfg.Logging.Level {
			defIdx = i
		}
	}
	idx, err := promptChoice(r, w, "Log level:", levels, defIdx)
	if err != nil {
		return err
	}
	cfg.Logging.Level = levels[idx]

	logFile, err := prompt(r, w, "Log file (empty = stderr only)", cfg.Logging.File)
	if err != nil {
		return err
	}
	cfg.Logging.File = logFile

	metricsOn, err := promptYesNo(r, w, "Enable Prometheus metrics endpoint?", cfg.Metrics.Enabled)
	if err != nil {
		return err
	}
	cfg.Metrics.Enabled = metricsOn
	if !metricsOn {
		return nil
	}
	cfg.Metrics.Addr, err = prompt(r, w, "Metrics listen address", orDefault(cfg.Metrics.Addr, "127.0.0.1:9101"))
	if err != nil {
		return err
	}
	return nil
}

// --- entry point ----------------------------------------------------------

// sectionFlow mutates cfg in place for one configuration area.
type sectionFlow func(cfg *config.Config, r *bufio.Reader, w io.Writer) error

// sectionDef pairs a section key (what the user types after `alga-agent setup`)
// with its label, flow, and a one-line status badge for the main menu.
type sectionDef struct {
	key    string
	label  string
	run    sectionFlow
	status func(*config.Config) string
}

// sections is the ordered registry. Order is stable so the main menu and the
// "Available:" hint are deterministic.
var sections = []sectionDef{
	{key: "model", label: "Model & Provider", run: setupModel, status: modelStatus},
	{key: "channel", label: "Channels", run: setupChannels, status: channelStatus},
	{key: "tools", label: "Tools", run: setupTools, status: toolsStatus},
	{key: "behavior", label: "Agent Behavior", run: setupBehavior, status: behaviorStatus},
	{key: "logging", label: "Logging & Metrics", run: setupLogging, status: loggingStatus},
}

// findSection returns the sectionDef for key, or ok=false.
func findSection(key string) (sectionDef, bool) {
	for _, s := range sections {
		if s.key == key {
			return s, true
		}
	}
	return sectionDef{}, false
}

// Run executes the wizard. section is "" for the full menu, or one of the
// section keys (see sections) for a direct jump to one area.
func Run(section string) error {
	return runWith(os.Stdin, os.Stdout, section)
}

// runWith is the testable core: all I/O flows through r and w.
func runWith(stdin io.Reader, stdout io.Writer, section string) error {
	w := stdout
	if !isInteractive() {
		printNonInteractiveGuidance(w)
		return errors.New("setup requires an interactive terminal (stdin is not a TTY)")
	}
	// Validate the section up front so a usage error isn't masked by a later
	// config-load or parse problem.
	if section != "" {
		if _, ok := findSection(section); !ok {
			printError(w, "Unknown setup section: "+section)
			printInfo(w, "Available: "+sectionKeys())
			return fmt.Errorf("unknown setup section %q", section)
		}
	}
	r := bufio.NewReader(stdin)

	dir := config.ResolveDataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create data dir %s: %w", dir, err)
	}
	path := config.DefaultPath("")

	// Load existing config or start from defaults so current values are shown
	// as defaults and re-runs are non-destructive.
	cfg, err := config.Load("")
	var backupPath string
	if err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			// File exists but failed to parse. Don't clobber silently: back it
			// up and offer to start fresh so the user isn't locked out.
			printWarning(w, fmt.Sprintf("Could not parse %s: %v", path, err))
			start, perr := promptYesNo(r, w, "Back it up and start from defaults?", true)
			if perr != nil {
				return perr
			}
			if !start {
				return fmt.Errorf("load existing config %s: %w", path, err)
			}
			backupPath = path + ".bak." + time.Now().Format("20060102_150405")
			if cerr := copyFile(path, backupPath); cerr != nil {
				return fmt.Errorf("back up %s: %w", path, cerr)
			}
			printInfo(w, "Backed up to "+backupPath)
			cfg = config.Default()
			printInfo(w, "Starting from defaults.")
		} else {
			cfg = config.Default()
			printInfo(w, "No existing config found; starting from defaults.")
		}
	} else if _, statErr := os.Stat(path); statErr == nil {
		// Back up before mutating.
		backupPath = path + ".bak." + time.Now().Format("20060102_150405")
		if err := copyFile(path, backupPath); err != nil {
			printWarning(w, fmt.Sprintf("Could not back up existing config: %v", err))
			backupPath = ""
		}
	}

	printBanner(w)

	abort := func(cause error) error {
		fmt.Fprintln(w)
		printWarning(w, "Setup cancelled.")
		if backupPath != "" {
			if rerr := copyFile(backupPath, path); rerr == nil {
				printInfo(w, "Restored previous config from backup.")
			}
		}
		return cause
	}

	switch section {
	case "":
		// Full menu loop: let the user visit multiple sections before saving.
		if err := runMenu(cfg, r, w, path, backupPath); err != nil {
			return abort(err)
		}
	default:
		s, _ := findSection(section)
		printSectionBanner(w, s.label)
		if err := s.run(cfg, r, w); err != nil {
			return abort(err)
		}
		// Direct section jump: run the review & save gate before finishing.
		saved, err := finalize(cfg, r, w, path, backupPath)
		if err != nil {
			return abort(err)
		}
		if !saved {
			// User chose to go back / fix — but a direct section jump has no
			// menu to return to, so treat it as a cancel.
			return abort(ErrAbort)
		}
	}
	return nil
}

// runMenu is the full-wizard top-level loop. The user visits sections (each
// showing a live status badge) and ends via "Review & Save" or "Exit without
// saving". Review & Save runs the validate gate (finalize) before writing.
func runMenu(cfg *config.Config, r *bufio.Reader, w io.Writer, path, backupPath string) error {
	defIdx := 0
	for {
		fmt.Fprintln(w)
		fmt.Fprintln(w, color("What do you want to configure?", colorBold))
		choices := mainMenuChoices(cfg)
		idx, err := promptChoice(r, w, "", choices, defIdx)
		if err != nil {
			return err
		}
		// Section entries are 0..len(sections)-1; the last two are the
		// Review/Save and Exit actions.
		if idx < len(sections) {
			s := sections[idx]
			printSectionBanner(w, s.label)
			if err := s.run(cfg, r, w); err != nil {
				return err
			}
			printSuccess(w, s.label+" updated.")
			defIdx = idx // keep cursor here for quick re-edits
			continue
		}
		switch idx {
		case len(sections): // Review & Save
			saved, ferr := finalize(cfg, r, w, path, backupPath)
			if ferr != nil {
				return ferr
			}
			if saved {
				return nil
			}
			// Not saved (validation failure → back to menu, or declined) — loop.
		case len(sections) + 1: // Exit without saving
			return ErrAbort
		}
	}
}

// mainMenuChoices builds the top-level menu rows: one per section with a live
// status badge, followed by the Review/Save and Exit actions.
func mainMenuChoices(cfg *config.Config) []string {
	choices := make([]string, 0, len(sections)+2)
	for _, s := range sections {
		choices = append(choices, fmt.Sprintf("%-20s %s", s.label, color(s.status(cfg), colorDim)))
	}
	choices = append(choices, "Review & Save")
	choices = append(choices, "Exit without saving")
	return choices
}

// finalize is the pre-save gate shared by the full menu and direct-section
// jumps. It shows a full review (secrets hidden), validates, and on success
// writes the config. Returns saved=true when the file was written.
func finalize(cfg *config.Config, r *bufio.Reader, w io.Writer, path, backupPath string) (bool, error) {
	printReview(w, cfg)

	if verr := cfg.Validate(); verr != nil {
		printError(w, "Configuration is not valid yet:")
		printInfo(w, verr.Error())
		choices := []string{"Back to menu (fix)", "Save anyway", "Cancel"}
		idx, err := promptChoice(r, w, "What now?", choices, 0)
		if err != nil {
			return false, err
		}
		switch idx {
		case 0:
			return false, nil // back to menu
		case 1:
			// fall through to save
		case 2:
			return false, ErrAbort
		}
	}

	confirm, err := promptYesNo(r, w, "Save configuration?", true)
	if err != nil {
		return false, err
	}
	if !confirm {
		printInfo(w, "Not saved. Returning to menu.")
		return false, nil
	}

	if err := config.Save(path, cfg); err != nil {
		return false, fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintln(w)
	printSuccess(w, "Configuration saved to "+path)
	if backupPath != "" {
		printInfo(w, "Previous config backed up to "+backupPath)
	}
	fmt.Fprintln(w)
	printInfo(w, "Run `alga-agent` to start the agent.")
	return true, nil
}

// printReview prints a comprehensive summary of every config area. Secrets are
// shown only as set/not-set — never their values.
func printReview(w io.Writer, cfg *config.Config) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, color("Review configuration", colorCyan+colorBold))

	// Model
	printInfo(w, "Provider:    "+cfg.Model.Provider)
	printInfo(w, "Base URL:    "+cfg.Model.BaseURL)
	printInfo(w, "Model:       "+cfg.Model.Model)
	printInfo(w, "Max tokens:  "+strconv.Itoa(cfg.Model.MaxTokens))
	printInfo(w, "Temperature: "+strconv.FormatFloat(cfg.Model.Temperature, 'f', -1, 64))
	printInfo(w, "API key:     "+secretStatus(cfg.Model.APIKey))

	// Channels
	tg := "off"
	if cfg.Telegram.Enabled {
		tg = "on"
	}
	ag := "off"
	if cfg.Alga.Enabled {
		ag = "on"
	}
	printInfo(w, "Telegram:    "+tg+"  (token: "+secretStatus(cfg.Telegram.BotToken)+")")
	printInfo(w, "Alga:        "+ag+"  (token: "+secretStatus(cfg.Alga.AgentToken)+")")

	// Tools
	shell := "off"
	if cfg.Tools.Shell.Enabled {
		shell = "on (" + strconv.Itoa(len(cfg.Tools.Shell.AllowedCommands)) + " commands)"
	}
	search := "off"
	if cfg.Tools.WebSearch.Enabled {
		search = "on (" + cfg.Tools.WebSearch.Provider + ")"
	}
	printInfo(w, "Shell:       "+shell)
	printInfo(w, "Web search:  "+search+"  (key: "+secretStatus(cfg.Tools.WebSearch.APIKey)+")")

	// Behavior
	printInfo(w, "Iterations:  "+strconv.Itoa(cfg.AgentBehavior.MaxIterations)+
		"  · timeout: "+cfg.AgentBehavior.ToolTimeout.String()+
		"  · ctx: "+strconv.Itoa(cfg.AgentBehavior.ContextWindow))

	// Logging & metrics
	metrics := "off"
	if cfg.Metrics.Enabled {
		metrics = "on (" + cfg.Metrics.Addr + ")"
	}
	printInfo(w, "Log level:   "+cfg.Logging.Level+"  (file: "+orDefault(cfg.Logging.File, "stderr")+")")
	printInfo(w, "Metrics:     "+metrics)
}

// secretStatus reports whether a secret is set without revealing its value.
func secretStatus(v string) string {
	if v == "" {
		return color("✗ not set", colorRed)
	}
	return color("✓ set", colorGreen)
}

func printBanner(w io.Writer) {
	fmt.Fprintln(w)
	border := "┌─────────────────────────────────────────────────────────┐"
	closeB := "└─────────────────────────────────────────────────────────┘"
	fmt.Fprintln(w, color(border, colorMag))
	fmt.Fprintln(w, color("│             ⚕ Alga Agent Setup                          │", colorMag))
	fmt.Fprintln(w, color("├─────────────────────────────────────────────────────────┤", colorMag))
	fmt.Fprintln(w, color("│  Configure model, channels, tools, and behavior.        │", colorMag))
	fmt.Fprintln(w, color("│  Press Enter to keep the [default]; Ctrl+C to exit.     │", colorMag))
	fmt.Fprintln(w, color(closeB, colorMag))
}

func printSectionBanner(w io.Writer, label string) {
	fmt.Fprintln(w)
	border := "┌─────────────────────────────────────────────────────────┐"
	closeB := "└─────────────────────────────────────────────────────────┘"
	fmt.Fprintln(w, color(border, colorMag))
	fmt.Fprintln(w, color(padLine("│     ⚕ Alga Agent Setup — "+label), colorMag))
	fmt.Fprintln(w, color(closeB, colorMag))
}

// padLine pads content to fit the 59-char banner width and closes with │.
func padLine(s string) string {
	const width = 59
	s = s + " "
	for len(s) < width-1 {
		s += " "
	}
	if len(s) > width-1 {
		s = s[:width-1]
	}
	return s + "│"
}

func printNonInteractiveGuidance(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, color("⚕ Alga Agent Setup — non-interactive mode", colorCyan+colorBold))
	fmt.Fprintln(w)
	printInfo(w, "The interactive wizard needs a TTY. Configure via environment variables instead:")
	printInfo(w, "  export OPENAI_API_KEY=\"sk-...\"")
	printInfo(w, "  export TELEGRAM_BOT_TOKEN=\"...\"     # if Telegram enabled")
	printInfo(w, "  export ALGA_TELEGRAM_ENABLED=true")
	printInfo(w, "Or copy apps/alga-agent/config.yaml.example to "+filepath.Join(config.ResolveDataDir(), "config.yaml")+" and edit it.")
	printInfo(w, "Run `alga-agent setup` in an interactive terminal for the full wizard.")
}

// --- small utilities ------------------------------------------------------

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func orNonZero(v, def int) int {
	if v != 0 {
		return v
	}
	return def
}

func orNonZeroF(v, def float64) float64 {
	if v != 0 {
		return v
	}
	return def
}

func orNonZeroDur(v, def time.Duration) time.Duration {
	if v != 0 {
		return v
	}
	return def
}

func sectionKeys() string {
	keys := make([]string, 0, len(sections))
	for _, s := range sections {
		keys = append(keys, s.key)
	}
	return strings.Join(keys, ", ")
}

// --- status badges --------------------------------------------------------
//
// Each returns a short, human-readable snapshot of one config area for the
// main menu rows. They read only; they never mutate cfg.

func modelStatus(cfg *config.Config) string {
	return cfg.Model.Model + " @ " + cfg.Model.Provider
}

func channelStatus(cfg *config.Config) string {
	parts := []string{
		"telegram " + onOff(cfg.Telegram.Enabled),
		"alga " + onOff(cfg.Alga.Enabled),
	}
	return strings.Join(parts, " · ")
}

func toolsStatus(cfg *config.Config) string {
	shell := "shell " + onOff(cfg.Tools.Shell.Enabled)
	search := "search off"
	if cfg.Tools.WebSearch.Enabled {
		search = "search on (" + cfg.Tools.WebSearch.Provider + ")"
	}
	return shell + " · " + search
}

func behaviorStatus(cfg *config.Config) string {
	return strconv.Itoa(cfg.AgentBehavior.MaxIterations) + " iters · " +
		strconv.Itoa(cfg.AgentBehavior.ContextWindow) + " ctx"
}

func loggingStatus(cfg *config.Config) string {
	metrics := "metrics off"
	if cfg.Metrics.Enabled {
		metrics = "metrics on"
	}
	return cfg.Logging.Level + " · " + metrics
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
