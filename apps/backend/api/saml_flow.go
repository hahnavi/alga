package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"

	"alga/valkey"
)

const (
	samlFlowTTL      = 10 * time.Minute
	samlReplayTTL    = 24 * time.Hour
	samlValkeyPrefix = "alga:saml:"
)

type samlLoginPayload struct {
	ProviderID string `json:"provider_id"`
	RequestID  string `json:"request_id"`
	NextPath   string `json:"next_path"`
}

type samlFlowStore interface {
	Set(ctx context.Context, relayState string, payload samlLoginPayload) error
	Consume(ctx context.Context, relayState string) (samlLoginPayload, bool, error)
	MarkAssertionUsed(ctx context.Context, issuer, assertionID string) (bool, error)
}

type valkeySAMLFlowStore struct {
	client *valkey.Client
}

func newValkeySAMLFlowStore(client *valkey.Client) samlFlowStore {
	return &valkeySAMLFlowStore{client: client}
}

func (s *valkeySAMLFlowStore) Set(ctx context.Context, relayState string, payload samlLoginPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal saml login state: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	created, err := s.client.SetNX(ctx, samlFlowKey(relayState), string(data), samlFlowTTL)
	if err != nil {
		return fmt.Errorf("store saml login state: %w", err)
	}
	if !created {
		return errors.New("saml login state collision")
	}
	return nil
}

func (s *valkeySAMLFlowStore) Consume(ctx context.Context, relayState string) (samlLoginPayload, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result := s.client.Do(ctx, s.client.Builder().Getdel().Key(samlFlowKey(relayState)).Build())
	raw, err := result.ToString()
	if errors.Is(err, valkeygo.Nil) {
		return samlLoginPayload{}, false, nil
	}
	if err != nil {
		return samlLoginPayload{}, false, fmt.Errorf("consume saml login state: %w", err)
	}
	var payload samlLoginPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return samlLoginPayload{}, false, fmt.Errorf("unmarshal saml login state: %w", err)
	}
	return payload, true, nil
}

func (s *valkeySAMLFlowStore) MarkAssertionUsed(ctx context.Context, issuer, assertionID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	used, err := s.client.SetNX(ctx, samlAssertionKey(issuer, assertionID), "1", samlReplayTTL)
	if err != nil {
		return false, fmt.Errorf("record saml assertion replay key: %w", err)
	}
	return used, nil
}

type memorySAMLFlowStore struct {
	mu         sync.Mutex
	flows      map[string]memorySAMLFlow
	assertions map[string]time.Time
}

type memorySAMLFlow struct {
	payload samlLoginPayload
	expires time.Time
}

func newMemorySAMLFlowStore() *memorySAMLFlowStore {
	return &memorySAMLFlowStore{
		flows:      make(map[string]memorySAMLFlow),
		assertions: make(map[string]time.Time),
	}
}

func (s *memorySAMLFlowStore) Set(_ context.Context, relayState string, payload samlLoginPayload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for key, flow := range s.flows {
		if !now.Before(flow.expires) {
			delete(s.flows, key)
		}
	}
	for key, expires := range s.assertions {
		if !now.Before(expires) {
			delete(s.assertions, key)
		}
	}
	if _, exists := s.flows[relayState]; exists {
		return errors.New("saml login state collision")
	}
	s.flows[relayState] = memorySAMLFlow{payload: payload, expires: now.Add(samlFlowTTL)}
	return nil
}

func (s *memorySAMLFlowStore) Consume(_ context.Context, relayState string) (samlLoginPayload, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	flow, ok := s.flows[relayState]
	if !ok {
		return samlLoginPayload{}, false, nil
	}
	delete(s.flows, relayState)
	if !time.Now().Before(flow.expires) {
		return samlLoginPayload{}, false, nil
	}
	return flow.payload, true, nil
}

func (s *memorySAMLFlowStore) MarkAssertionUsed(_ context.Context, issuer, assertionID string) (bool, error) {
	key := samlAssertionKey(issuer, assertionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if expires, exists := s.assertions[key]; exists && now.Before(expires) {
		return false, nil
	}
	s.assertions[key] = now.Add(samlReplayTTL)
	return true, nil
}

func samlFlowKey(relayState string) string {
	return samlValkeyPrefix + "flow:" + relayState
}

func samlAssertionKey(issuer, assertionID string) string {
	key := sha256.Sum256([]byte(issuer + "\x00" + assertionID))
	return samlValkeyPrefix + "assertion:" + hex.EncodeToString(key[:])
}

func newSAMLRelayState() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate saml relay state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func validSAMLRelayState(value string) bool {
	if len(value) != 43 || strings.ContainsAny(value, "+/=") {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}
