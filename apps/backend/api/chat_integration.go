// Code moved from http.go; see git history.

package api

import (
	"net/http"
	"os"
	"strconv"

	"alga/logger"
	"alga/mattermost"
	"alga/slack"
	"alga/store"
	"alga/webhook"
)

func buildChatRouter(mmClient *mattermost.Client, slackClient *slack.Client) *webhook.ChatRouter {
	var providers []webhook.ChatProvider
	if mmClient != nil {
		providers = append(providers, webhook.NewMattermostChatProvider(mmClient))
	}
	if slackClient != nil {
		providers = append(providers, webhook.NewSlackChatProvider(slackClient))
	}
	return webhook.NewChatRouter(providers...)
}

func (s *Server) rebuildChatRouter() {
	router := buildChatRouter(s.mmClient, s.slackClient)
	s.mu.Lock()
	s.chatRouter = router
	s.mu.Unlock()
	if s.chatSync != nil {
		s.chatSync.Rebuild(s.mmClient, s.slackClient)
	}
}

func (s *Server) handleSlackOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.slackOAuthHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "Slack OAuth not available")
		return
	}
	s.slackOAuthHandler.handleAuthorize(w, r)
}

func (s *Server) handleSlackOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.slackOAuthHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "Slack OAuth not available")
		return
	}
	s.slackOAuthHandler.handleCallback(w, r)
}

func (s *Server) handleSlackDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if s.integrationStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "integration store not available")
		return
	}
	existing, err := s.integrationStore.Get()
	if err != nil || existing == nil {
		writeInternalError(w, err, "failed to load integrations")
		return
	}
	existing.SlackBotToken = ""
	existing.SlackWorkspaceName = ""
	existing.SlackWorkspaceID = ""
	if err := s.integrationStore.Save(*existing); err != nil {
		writeInternalError(w, err, "failed to disconnect Slack")
		return
	}
	s.mu.Lock()
	s.cfg.SlackBotToken = ""
	s.cfg.SlackWorkspaceName = ""
	s.cfg.SlackWorkspaceID = ""
	s.mu.Unlock()
	if s.slackClient != nil {
		s.slackClient.Reconfigure("")
	}
	s.rebuildChatRouter()
	s.audit(r, store.AuditSlackDisconnected, map[string]any{
		"workspace_id": existing.SlackWorkspaceID,
		"workspace":    existing.SlackWorkspaceName,
	})
	writeStatus(w, "disconnected")
}

// handleMMPluginDownload serves the Mattermost plugin tarball for internal use
// by the alga-plugin-ensure CronJob. No auth required — only reachable within cluster.
func (s *Server) handleMMPluginDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	path := "/app/plugins/com.alga.mattermost-plugin-0.0.1.tar.gz"
	f, err := os.Open(path)
	if err != nil {
		logger.Error("mm-plugin: tarball not found", "path", path, "error", err)
		http.Error(w, "plugin tarball not found", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "stat error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=com.alga.mattermost-plugin-0.0.1.tar.gz")
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	http.ServeContent(w, r, "com.alga.mattermost-plugin-0.0.1.tar.gz", stat.ModTime(), f)
}
