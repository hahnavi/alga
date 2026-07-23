package sse

import (
	"os"
	"testing"

	"alga/logger"
)

func TestMain(m *testing.M) {
	logger.Init("fatal", "")
	os.Exit(m.Run())
}

func TestBroker_PublishToNoClients(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	b.Publish(Event{Type: "test"})
}

func TestBroker_PublishReceivesEvent(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	ch := make(chan Event, 1)
	b.mu.Lock()
	b.clients["c1"] = ch
	b.mu.Unlock()

	b.Publish(Event{Type: "alert", Data: "hello"})

	select {
	case e := <-ch:
		if e.Type != "alert" {
			t.Fatalf("expected type alert, got %s", e.Type)
		}
		if e.ID == "" {
			t.Fatal("expected auto-generated ID")
		}
		if e.Data != "hello" {
			t.Fatalf("expected data hello, got %v", e.Data)
		}
	default:
		t.Fatal("expected to receive event")
	}
}

func TestBroker_PublishDropsOnFullBuffer(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	ch := make(chan Event, 1)
	b.mu.Lock()
	b.clients["c1"] = ch
	b.mu.Unlock()

	b.Publish(Event{Type: "first"})
	b.Publish(Event{Type: "second"})

	e := <-ch
	if e.Type != "first" {
		t.Fatalf("expected first event, got %s", e.Type)
	}
}

func TestBroker_SubscribeUnsubscribeUser(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	userID := "u1"
	ch := make(chan Event, 1)

	b.SubscribeUser(userID, "c1", ch)

	b.PublishToUser(userID, Event{Type: "hello"})
	select {
	case e := <-ch:
		if e.Type != "hello" {
			t.Fatalf("expected hello, got %s", e.Type)
		}
	default:
		t.Fatal("expected event while subscribed")
	}

	b.UnsubscribeUser(userID, "c1")
	b.PublishToUser(userID, Event{Type: "after"})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("should not receive event after unsubscribe")
		}
	default:
	}
}

func TestBroker_PublishToUnknownUser(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	b.PublishToUser("nonexistent", Event{Type: "test"})
}

func TestBroker_AgentOnline(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	agentID := "agent-1"
	ch := make(chan Event, 1)

	if b.AgentOnline(agentID) {
		t.Fatal("expected agent to be offline before subscribe")
	}

	b.SubscribeAgent(agentID, "c1", ch)

	if !b.AgentOnline(agentID) {
		t.Fatal("expected agent to be online after subscribe")
	}

	b.UnsubscribeAgent(agentID, "c1")

	if b.AgentOnline(agentID) {
		t.Fatal("expected agent to be offline after unsubscribe")
	}
}

func TestBroker_BroadcastToAgents(t *testing.T) {
	t.Parallel()
	b := NewBroker()

	ch1 := make(chan Event, 1)
	ch2 := make(chan Event, 1)
	ch3 := make(chan Event, 1)

	b.SubscribeAgent("a1", "c1", ch1)
	b.SubscribeAgent("a2", "c1", ch2)
	b.SubscribeAgent("a3", "c1", ch3)

	b.BroadcastToAgents(Event{Type: "broadcast"}, "a2")

	assertReceived := func(ch chan Event, label string) {
		select {
		case e := <-ch:
			if e.Type != "broadcast" {
				t.Fatalf("%s: expected broadcast, got %s", label, e.Type)
			}
		default:
			t.Fatalf("%s: expected event", label)
		}
	}

	assertReceived(ch1, "a1")
	assertReceived(ch3, "a3")

	select {
	case <-ch2:
		t.Fatal("a2 should have been excluded")
	default:
	}
}

func TestBroker_PublishToAgent_ReturnsErrorWhenNoClients(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	err := b.PublishToAgent("ghost-agent", Event{Type: "test"})
	if err == nil {
		t.Fatal("expected error when no clients")
	}
}

func TestBroker_PublishToAgentAllowDrop_NoClients(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	b.PublishToAgentAllowDrop("ghost-agent", Event{Type: "test"})
}
