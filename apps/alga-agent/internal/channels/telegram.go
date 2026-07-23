package channels

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"alga-agent/internal/config"
)

// TelegramChannel adapts the Telegram Bot API to the Channel/ResponseSink
// interfaces. It supports long polling (default) and optional webhooks.
type TelegramChannel struct {
	api     TelegramAPI
	cfg     config.TelegramConfig
	router  *Router
	logger  *slog.Logger
	allowed map[string]struct{}

	// botUsername is the bot's own @username (without @), used to detect
	// mentions in group chats. Populated at Start via GetMe.
	botUsername string

	// outbound message state per chat (for streaming edits).
	streamMu     sync.Mutex
	streamStates map[int64]*tgStreamState

	// webhook server (nil in long-polling mode).
	webhookSrv *webhookServer
	// stop signals the update loop to exit.
	stop context.CancelFunc
	// updates channel from the API; closed when stopped.
	updates tg.UpdatesChannel
	// stopped guards Stop() from double-invoking the API stop.
	stopped sync.Once
}

// TelegramAPI is the subset of *tg.BotAPI used by TelegramChannel. Tests may
// substitute a fake.
type TelegramAPI interface {
	GetMe() (tg.User, error)
	Send(c tg.Chattable) (tg.Message, error)
	GetUpdatesChan(config tg.UpdateConfig) tg.UpdatesChannel
	StopReceivingUpdates()
	Request(c tg.Chattable) (*tg.APIResponse, error)
}

// tgStreamState tracks one in-progress streaming message for rate-limit-aware
// progressive edits (SPEC §6.3, §8.3).
type tgStreamState struct {
	chatID    int64
	messageID int
	// lastEditAt bounds edit frequency to MinEditInterval.
	lastEditAt time.Time
	// rateLimitedUntil is set when a 429 is received; edits are skipped until.
	rateLimitedUntil time.Time
	// rateLimitStart is when the current rate-limiting episode began, to detect
	// the 60s threshold (SPEC §8.3).
	rateLimitStart time.Time
	// finalized is set when the message has been finalized and further edits
	// should be skipped for the turn.
	finalized bool
}

// NewTelegramChannel constructs a TelegramChannel. Returns (nil, nil) when
// Telegram is disabled in config.
func NewTelegramChannel(cfg config.TelegramConfig, router *Router, logger *slog.Logger) (*TelegramChannel, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	bot, err := tg.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("telegram bot init: %w", err)
	}
	return newTelegramChannelWithAPI(bot, cfg, router, logger)
}

// newTelegramChannelWithAPI allows injecting a fake TelegramAPI (tests).
func newTelegramChannelWithAPI(api TelegramAPI, cfg config.TelegramConfig, router *Router, logger *slog.Logger) (*TelegramChannel, error) {
	if logger == nil {
		logger = slog.Default()
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedUsers))
	for _, u := range cfg.AllowedUsers {
		allowed[strings.TrimSpace(u)] = struct{}{}
	}
	if cfg.MinEditInterval == 0 {
		cfg.MinEditInterval = time.Second
	}
	return &TelegramChannel{
		api:          api,
		cfg:          cfg,
		router:       router,
		logger:       logger,
		allowed:      allowed,
		streamStates: make(map[int64]*tgStreamState),
	}, nil
}

// Name implements Channel.
func (t *TelegramChannel) Name() string { return "telegram" }

// Start begins receiving messages. In webhook mode, starts an HTTP server;
// otherwise uses long polling.
func (t *TelegramChannel) Start(ctx context.Context) error {
	me, err := t.api.GetMe()
	if err != nil {
		return fmt.Errorf("telegram getMe: %w", err)
	}
	t.botUsername = me.UserName
	t.logger.Info("telegram bot connected", "username", "@"+me.UserName, "id", me.ID)

	if t.cfg.WebhookURL != "" {
		return t.startWebhook(ctx)
	}
	return t.startLongPolling(ctx)
}

func (t *TelegramChannel) startLongPolling(ctx context.Context) error {
	innerCtx, cancel := context.WithCancel(ctx)
	t.stop = cancel

	uc := tg.NewUpdate(0)
	uc.Timeout = 30
	t.updates = t.api.GetUpdatesChan(uc)

	go func() {
		for {
			select {
			case <-innerCtx.Done():
				return
			case upd, ok := <-t.updates:
				if !ok {
					return
				}
				t.handleUpdate(innerCtx, upd)
			}
		}
	}()
	return nil
}

func (t *TelegramChannel) startWebhook(ctx context.Context) error {
	innerCtx, cancel := context.WithCancel(ctx)
	t.stop = cancel

	// Derive a secret path from the token (constant-time compare on receive).
	path := "/" + t.cfg.BotToken

	// Construct the server synchronously so close() observes a fully
	// initialized *http.Server with no data race.
	t.webhookSrv = newWebhookServer(t.cfg.WebhookAddr, path, t.cfg.BotToken, func(u tg.Update) {
		t.handleUpdate(innerCtx, u)
	})

	// Register the webhook with Telegram.
	whCfg, err := tg.NewWebhook(t.cfg.WebhookURL)
	if err != nil {
		t.logger.Warn("telegram webhook config invalid, falling back to long polling", "err", err)
		return t.startLongPolling(ctx)
	}
	if _, err := t.api.Send(whCfg); err != nil {
		t.logger.Warn("telegram webhook registration failed, falling back to long polling", "err", err)
		return t.startLongPolling(ctx)
	}

	go t.webhookSrv.listen(t.logger)
	return nil
}

// handleUpdate processes a single Telegram update.
func (t *TelegramChannel) handleUpdate(ctx context.Context, upd tg.Update) {
	msg := upd.Message
	if msg == nil || msg.Text == "" {
		return
	}
	chat := msg.Chat
	if chat == nil {
		return
	}
	// Group handling: respond only when @mentioned or replied to (SPEC §4.1).
	if chat.Type != "private" && !t.cfg.RespondInGroups {
		if !t.isMentionedOrReplied(msg) {
			return
		}
	}
	// Strip the @mention from the text so it isn't passed to the agent.
	text := stripMention(msg.Text, t.botUsername)

	// Allowlist check.
	if len(t.allowed) > 0 {
		from := msg.From
		var senderKey string
		if from != nil {
			senderKey = fmt.Sprintf("%d", from.ID)
		}
		if _, ok := t.allowed[senderKey]; !ok {
			// Also allow username-based allowlist entries.
			if from == nil {
				return
			}
			if _, ok := t.allowed["@"+strings.TrimPrefix(from.UserName, "@")]; !ok {
				t.logger.Info("telegram message from non-allowlisted user", "user_id", from.ID, "username", from.UserName)
				return
			}
		}
	}

	senderName := ""
	if msg.From != nil {
		senderName = strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
		if senderName == "" {
			senderName = "@" + msg.From.UserName
		}
	}

	// Handle slash commands locally.
	if isCmd, handled := t.handleCommand(ctx, msg, text); isCmd {
		if !handled {
			// Command failed; nothing more to do.
		}
		return
	}

	sessionID := SessionIDFor("telegram", chat.ID)
	t.router.DispatchAsync(ctx, InboundMessage{
		SessionID:   sessionID,
		ChatID:      fmt.Sprintf("%d", chat.ID),
		Text:        text,
		SenderID:    senderKey(msg),
		SenderName:  senderName,
		ChannelName: t.Name(),
	})
}

func senderKey(msg *tg.Message) string {
	if msg.From == nil {
		return ""
	}
	return fmt.Sprintf("%d", msg.From.ID)
}

// isMentionedOrReplied reports whether the bot was @mentioned in the message
// text or the message is a reply to one of the bot's messages.
func (t *TelegramChannel) isMentionedOrReplied(msg *tg.Message) bool {
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.UserName == t.botUsername {
		return true
	}
	if t.botUsername != "" && strings.Contains(msg.Text, "@"+t.botUsername) {
		return true
	}
	// The message.Entities offsets are UTF-16 code units, not Go bytes, so we
	// can't safely index msg.Text by ent.Offset for non-ASCII messages. The
	// strings.Contains check above already handles mention detection correctly
	// for all encodings, so the entity loop is intentionally omitted.
	return false
}

// handleCommand processes slash commands locally (/start, /stop, /clear, /help).
// Returns (true, ok) when text is a command; (false, false) otherwise.
func (t *TelegramChannel) handleCommand(ctx context.Context, msg *tg.Message, text string) (bool, bool) {
	if !strings.HasPrefix(text, "/") {
		return false, false
	}
	cmd := strings.Fields(text)
	if len(cmd) == 0 {
		return false, false
	}
	name := strings.TrimPrefix(cmd[0], "/")
	// Telegram commands may carry @botusername suffix.
	name = strings.Split(name, "@")[0]

	chatID := msg.Chat.ID
	switch name {
	case "start":
		_, _ = t.sendText(chatID, "Hi! I'm "+defaultAgentName+". Send me a message and I'll help you triage alerts and manage incidents.")
		return true, true
	case "stop":
		_, _ = t.sendText(chatID, "Okay, I'll go quiet. Send me a message any time to resume.")
		return true, true
	case "clear":
		t.router.agent.Store().Clear(SessionIDFor("telegram", chatID))
		_, _ = t.sendText(chatID, "Conversation cleared.")
		return true, true
	case "help":
		_, _ = t.sendText(chatID, helpText())
		return true, true
	}
	return false, false
}

// --- ResponseSink implementation ---

// OnThinking sends an initial "thinking..." placeholder that will be edited
// progressively as the agent streams its response.
func (t *TelegramChannel) OnThinking(ctx context.Context, chatID string) error {
	id, err := parseChatID(chatID)
	if err != nil {
		return err
	}
	// Send chat action "typing".
	_, _ = t.api.Send(tg.NewChatAction(id, tg.ChatTyping))
	// Create the placeholder message.
	msg, err := t.api.Send(tg.MessageConfig{
		BaseChat: tg.BaseChat{ChatID: id},
		Text:     "…",
	})
	if err != nil {
		return err
	}
	t.streamMu.Lock()
	t.streamStates[id] = &tgStreamState{chatID: id, messageID: msg.MessageID, lastEditAt: time.Now()}
	t.streamMu.Unlock()
	return nil
}

// OnDelta edits the streaming placeholder with the accumulated text, throttled
// to MinEditInterval. Handles Telegram 429 rate limits per SPEC §8.3.
func (t *TelegramChannel) OnDelta(ctx context.Context, chatID, accumulated, delta string) bool {
	id, err := parseChatID(chatID)
	if err != nil {
		return true
	}
	t.streamMu.Lock()
	st, ok := t.streamStates[id]
	t.streamMu.Unlock()
	if !ok || st.finalized {
		// No placeholder to edit (channel may have lost the state). Continue
		// streaming without edits.
		return true
	}
	now := time.Now()
	if now.Before(st.rateLimitedUntil) {
		// Still in rate-limit backoff; skip this edit.
		return true
	}
	if now.Sub(st.lastEditAt) < t.cfg.MinEditInterval {
		// Throttle edits.
		return true
	}
	st.lastEditAt = now

	text := truncateForTelegram(accumulated)
	edit := tg.EditMessageTextConfig{
		BaseEdit: tg.BaseEdit{ChatID: id, MessageID: st.messageID},
		Text:     text, ParseMode: "Markdown",
	}
	if _, err := t.api.Send(edit); err != nil {
		if isRateLimited(err) {
			t.handleRateLimit(id, st, err)
			return true
		}
		// Non-rate-limit error: log and continue (the final edit may still work).
		t.logger.Debug("telegram edit failed", "err", err)
	}
	return true
}

// OnFinal delivers the completed response, editing the placeholder or sending
// a fresh message if none exists. It first attempts Markdown rendering, then
// falls back to plain text if Telegram rejects the formatting — arbitrary LLM
// output frequently contains stray `_` or `*` that breaks Markdown parsing.
func (t *TelegramChannel) OnFinal(ctx context.Context, chatID, text string) error {
	id, err := parseChatID(chatID)
	if err != nil {
		return err
	}
	t.streamMu.Lock()
	st, ok := t.streamStates[id]
	delete(t.streamStates, id)
	t.streamMu.Unlock()

	text = truncateForTelegram(text)

	if ok {
		edit := tg.EditMessageTextConfig{
			BaseEdit: tg.BaseEdit{ChatID: id, MessageID: st.messageID},
			Text:     text, ParseMode: "Markdown",
		}
		if _, err := t.api.Send(edit); err != nil {
			if isRateLimited(err) {
				// Wait the retry_after (or a short default) then retry once.
				backoff := retryAfterFromError(err)
				if backoff <= 0 {
					backoff = 2 * time.Second
				}
				time.Sleep(backoff)
				if _, err2 := t.api.Send(edit); err2 == nil {
					return nil
				}
				// Fall through to plain-text fallback.
			}
			// Markdown edit failed (parse error or unchanged): retry as plain text.
			plainEdit := edit
			plainEdit.ParseMode = ""
			if _, err2 := t.api.Send(plainEdit); err2 != nil {
				_, _ = t.sendText(id, text)
			}
			return nil
		}
		return nil
	}
	_, err = t.sendText(id, text)
	return err
}

// OnError delivers an error message to the user.
func (t *TelegramChannel) OnError(ctx context.Context, chatID, text string) error {
	id, err := parseChatID(chatID)
	if err != nil {
		return err
	}
	// Clear the streaming placeholder so it doesn't linger.
	t.streamMu.Lock()
	delete(t.streamStates, id)
	t.streamMu.Unlock()
	_, err = t.sendText(id, text)
	return err
}

// handleRateLimit applies the Telegram 429 backoff (SPEC §8.3). It honors the
// retry_after value returned by Telegram (parsed from the tg.Error), falling
// back to a 10s default. If rate limiting persists for 60s during a turn, the
// message is finalized once and further edits skipped for that turn.
func (t *TelegramChannel) handleRateLimit(chatID int64, st *tgStreamState, err error) {
	now := time.Now()
	if st.rateLimitStart.IsZero() {
		st.rateLimitStart = now
	}
	// Extract retry_after from the Telegram error (default 10s).
	backoff := 10 * time.Second
	if retryAfter := retryAfterFromError(err); retryAfter > 0 {
		backoff = retryAfter
	}
	st.rateLimitedUntil = now.Add(backoff)

	if now.Sub(st.rateLimitStart) >= 60*time.Second {
		// Persisted for >60s: finalize once, skip further edits.
		st.finalized = true
		t.logger.Warn("telegram rate limit persisted 60s, finalizing message", "chat_id", chatID)
	}
}

// retryAfterFromError extracts the Retry-After duration from a Telegram API
// error. Returns 0 if the error is not a tg.Error or has no retry_after.
func retryAfterFromError(err error) time.Duration {
	if err == nil {
		return 0
	}
	var apiErr tg.Error
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	return 0
}

func (t *TelegramChannel) sendText(chatID int64, text string) (tg.Message, error) {
	// Try Markdown first; on parse failure, fall back to plain text so the
	// message is never silently dropped.
	msg, err := t.api.Send(tg.MessageConfig{
		BaseChat: tg.BaseChat{ChatID: chatID},
		Text:     text, ParseMode: "Markdown",
	})
	if err != nil && isParseError(err) {
		return t.api.Send(tg.MessageConfig{
			BaseChat: tg.BaseChat{ChatID: chatID},
			Text:     text,
		})
	}
	return msg, err
}

// isParseError reports whether err is a Telegram "can't parse entities" error,
// which indicates invalid Markdown that should trigger a plain-text fallback.
func isParseError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "can't parse") || strings.Contains(msg, "entities")
}

// Stop shuts down the channel.
func (t *TelegramChannel) Stop() error {
	t.stopped.Do(func() {
		if t.stop != nil {
			t.stop()
		}
		if t.cfg.WebhookURL == "" && t.updates != nil {
			t.api.StopReceivingUpdates()
		}
		if t.webhookSrv != nil {
			_ = t.webhookSrv.close()
		}
	})
	return nil
}

// --- helpers ---

func parseChatID(s string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid telegram chat id %q: %w", s, err)
	}
	return id, nil
}

// isRateLimited returns true for Telegram 429 errors.
func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	var apiErr tg.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429
	}
	// Fall back to string check for wrapped errors.
	return strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "Too Many Requests")
}

// stripMention removes the @botname mention from text.
func stripMention(text, username string) string {
	if username == "" {
		return strings.TrimSpace(text)
	}
	cleaned := strings.ReplaceAll(text, "@"+username, "")
	return strings.TrimSpace(cleaned)
}

// truncateForTelegram bounds text to Telegram's 4096-char message limit. It
// prefers to truncate at a newline boundary and snaps to a valid UTF-8 rune
// boundary to avoid emitting invalid UTF-8.
func truncateForTelegram(text string) string {
	const maxLen = 4000 // leave headroom for Markdown formatting.
	if len(text) <= maxLen {
		return text
	}
	// Find the last newline before the limit.
	cut := maxLen
	if i := strings.LastIndex(text[:maxLen], "\n"); i > maxLen/2 {
		cut = i
	}
	// Snap backward to a valid UTF-8 rune boundary so we don't split a
	// multibyte character (emoji, accented letters — common in SRE output).
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "\n\n…(truncated)"
}

func helpText() string {
	return strings.Join([]string{
		"*Alga Agent* — your SRE assistant.",
		"",
		"I can help you triage alerts, investigate incidents, and run operations.",
		"",
		"Commands:",
		"/start — greet me",
		"/clear — clear conversation history",
		"/help — show this help",
		"/stop — go quiet",
		"",
		"Just send me a message to get started.",
	}, "\n")
}

const defaultAgentName = "Alga Agent"
