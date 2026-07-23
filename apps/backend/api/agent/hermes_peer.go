package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"alga/logger"
	"alga/sse"
	"alga/valkey"
)

type PeerFindingFrame struct {
	Type            string            `json:"type"`
	InvestigationID string            `json:"investigation_id"`
	PeerAgentID     string            `json:"peer_agent_id,omitempty"`
	PeerAgentType   string            `json:"peer_agent_type,omitempty"`
	Text            string            `json:"text"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type PeerAskFrame struct {
	Type            string    `json:"type"`
	AskID           string    `json:"ask_id"`
	FromAgentID     string    `json:"from_agent_id"`
	FromAgentName   string    `json:"from_agent_name"`
	FromAgentType   string    `json:"from_agent_type"`
	InvestigationID string    `json:"investigation_id,omitempty"`
	Question        string    `json:"question"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type PeerReplyFrame struct {
	Type               string    `json:"type"`
	AskID              string    `json:"ask_id"`
	InvestigationID    string    `json:"investigation_id,omitempty"`
	Reply              string    `json:"reply"`
	RepliedByAgentID   string    `json:"replied_by_agent_id,omitempty"`
	RepliedByAgentName string    `json:"replied_by_agent_name,omitempty"`
	AnsweredAt         time.Time `json:"answered_at"`
}

func BroadcastPeerFinding(agentSSE *AgentSSEHandler, f valkey.PeerFinding) {
	if agentSSE == nil || strings.TrimSpace(f.Text) == "" {
		return
	}
	frame := PeerFindingFrame{
		Type:            "peer_finding",
		InvestigationID: f.InvestigationID,
		PeerAgentID:     f.AgentID,
		PeerAgentType:   f.AgentType,
		Text:            f.Text,
		Labels:          f.Labels,
		CreatedAt:       f.CreatedAt,
	}
	event := sse.Event{Type: "peer_finding", Data: frame}
	agentSSE.BroadcastToAgents(event, f.AgentID)

	if agentSSE.vkClient != nil {
		data, _ := json.Marshal(frame)
		if len(data) > 0 {
			if err := sse.PublishToValkey(context.Background(), agentSSE.vkClient.Client(), sse.Event{Type: "peer_finding", Data: json.RawMessage(data)}); err != nil {
				logger.Warn("Failed to publish peer finding to Valkey SSE", "error", err)
			}
		}
	}

	logger.Debug("Broadcast peer finding from agent to all agents", "agent_id", f.AgentID)
}

func PublishPeerAskToAgent(agentSSE *AgentSSEHandler, agentKey string, frame PeerAskFrame) {
	if agentSSE == nil {
		return
	}
	event := sse.Event{Type: "peer_ask", Data: frame}
	agentSSE.PublishToAgentAllowDrop(agentKey, event)
}

func BroadcastPeerAskToType(agentSSE *AgentSSEHandler, frame PeerAskFrame) {
	if agentSSE == nil {
		return
	}
	event := sse.Event{Type: "peer_ask", Data: frame}
	agentSSE.BroadcastToAgents(event, "")
}

func PublishPeerReplyToAgent(agentSSE *AgentSSEHandler, agentKey string, frame PeerReplyFrame) {
	if agentSSE == nil {
		return
	}
	event := sse.Event{Type: "peer_reply", Data: frame}
	agentSSE.PublishToAgentAllowDrop(agentKey, event)
}
