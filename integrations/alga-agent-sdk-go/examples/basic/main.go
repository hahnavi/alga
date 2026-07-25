package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	alga "github.com/alga/agent-sdk-go"
)

func main() {
	serverURL := os.Getenv("ALGA_SERVER_URL")
	token := os.Getenv("ALGA_AGENT_TOKEN")
	if serverURL == "" || token == "" {
		fmt.Fprintln(os.Stderr, "ALGA_SERVER_URL and ALGA_AGENT_TOKEN are required")
		os.Exit(1)
	}

	client := alga.NewAlgaClient(serverURL, token)

	client.OnConnected = func(evt alga.ConnectedEvent) {
		fmt.Printf("Connected as agent %s (session %s)\n", evt.AgentID, evt.ClientID)
	}

	client.OnMessage = func(evt alga.MessageEvent) {
		fmt.Printf("Message in %s from %s: %s\n", evt.ChatID, evt.SenderName, evt.Text)
		if evt.Trigger == "observe" {
			// Context-only delivery; do not reply.
			return
		}

		resp, err := client.SendMessage(context.Background(), evt.ChatID, "Acknowledged, investigating...", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send reply: %v\n", err)
			return
		}
		fmt.Printf("Reply sent (message_id=%s)\n", resp.MessageID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		fmt.Println("Shutting down...")
	case err := <-client.Err():
		fmt.Fprintf(os.Stderr, "Terminal error (token revoked?): %v\n", err)
	}
	client.Disconnect()
}
