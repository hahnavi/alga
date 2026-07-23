package triage

import (
	"context"
	"fmt"
	"time"

	"alga/config"
	"alga/logger"
	"alga/rabbitmq"
	"alga/routing"
	"alga/store"
)

type EpisodicContextFinder interface {
	Find(ctx context.Context, correlationKey string, limit int) []EpisodicEntry
}

type NotesContextFinder interface {
	Find(ctx context.Context, labels map[string]string, limit int) []NoteEntry
}

type MemoryContextFinder interface {
	Find(ctx context.Context, correlationKey string, labels map[string]string, limit int) []MemoryEntry
}

type ConcurrentContextFinder interface {
	Count(ctx context.Context, correlationKey string) int
}

type Engine struct {
	cfg               *config.Config
	ruleEvaluator     *RuleEvaluator
	llmClient         LLMClient
	publisher         *rabbitmq.Publisher
	triageResultStore store.TriageResultStore

	episodicFinder   EpisodicContextFinder
	notesFinder      NotesContextFinder
	memoryFinder     MemoryContextFinder
	concurrentFinder ConcurrentContextFinder
}

func NewEngine(
	cfg *config.Config,
	ruleEvaluator *RuleEvaluator,
	llmClient LLMClient,
	publisher *rabbitmq.Publisher,
	triageResultStore store.TriageResultStore,
) *Engine {
	return &Engine{
		cfg:               cfg,
		ruleEvaluator:     ruleEvaluator,
		llmClient:         llmClient,
		publisher:         publisher,
		triageResultStore: triageResultStore,
	}
}

func (e *Engine) SetEpisodicFinder(f EpisodicContextFinder)     { e.episodicFinder = f }
func (e *Engine) SetNotesFinder(f NotesContextFinder)           { e.notesFinder = f }
func (e *Engine) SetMemoryFinder(f MemoryContextFinder)         { e.memoryFinder = f }
func (e *Engine) SetConcurrentFinder(f ConcurrentContextFinder) { e.concurrentFinder = f }

type TriageResultWrapper struct {
	Record   *store.TriageResultRecord
	Response *TriageResponse
}

func (e *Engine) Process(ctx context.Context, msg rabbitmq.TriageMessage) (*TriageResultWrapper, error) {
	start := time.Now()

	labelMaps := make([]map[string]string, len(msg.Alerts))
	for i, a := range msg.Alerts {
		labelMaps[i] = a.Labels
	}
	commonLabels := routing.FindCommonKeyValues(labelMaps)

	annotationMaps := make([]map[string]string, len(msg.Alerts))
	for i, a := range msg.Alerts {
		annotationMaps[i] = a.Annotations
	}
	commonAnnotations := routing.FindCommonKeyValues(annotationMaps)

	ruleMatch, err := e.ruleEvaluator.Evaluate(ctx, commonLabels, commonAnnotations)
	if err != nil {
		logger.WarnCtx(ctx, "Rule evaluator failed, falling through to LLM", "component", "triage", "error", err)
	}
	if ruleMatch != nil {
		record := e.buildRecord(msg, commonLabels, ruleMatch.Decision, 1.0, ruleMatch.Severity, ruleMatch.Category, "Deterministic rule match: "+ruleMatch.Rule.Name, nil, "", start)
		record.Enrichment = ruleMatch.Enrichment
		saved, err := e.triageResultStore.Create(ctx, record)
		if err != nil {
			return nil, fmt.Errorf("save rule triage result: %w", err)
		}
		return &TriageResultWrapper{Record: saved, Response: &TriageResponse{Decision: ruleMatch.Decision}}, nil
	}

	if e.llmClient != nil && e.cfg.TriageLLMURL != "" {
		input := e.gatherContext(ctx, msg, commonLabels)
		sysPrompt, userPrompt := BuildTriagePrompt(input)
		raw, err := e.llmClient.Generate(ctx, sysPrompt, userPrompt)
		if err != nil {
			logger.WarnCtx(ctx, "LLM triage call failed, falling back to enrich_only", "component", "triage", "error", err)
		} else {
			resp, err := ParseTriageResponse(raw)
			if err != nil {
				logger.WarnCtx(ctx, "Failed to parse LLM triage response, falling back to enrich_only", "component", "triage", "error", err)
			} else {
				decision := resp.Decision
				if (decision == store.TriageDecisionAutoResolve && !e.cfg.TriageAutoResolveEnabled) ||
					(decision == store.TriageDecisionSuppress && !e.cfg.TriageSuppressEnabled) ||
					(resp.Confidence < e.cfg.TriageConfidenceThreshold && decision != store.TriageDecisionInvestigate) {
					decision = store.TriageDecisionEnrichOnly
				}
				enrich := map[string]any{
					"service_owner":   resp.Enrichment.ServiceOwner,
					"runbook_url":     resp.Enrichment.RunbookURL,
					"past_root_cause": resp.Enrichment.PastRootCause,
					"past_resolution": resp.Enrichment.PastResolution,
					"custom":          resp.Enrichment.Custom,
				}
				record := e.buildRecord(msg, commonLabels, decision, resp.Confidence, resp.Severity, resp.Category, resp.Reasoning, resp.SuggestedActions, e.cfg.TriageLLMModel, start)
				record.Enrichment = enrich
				saved, err := e.triageResultStore.Create(ctx, record)
				if err != nil {
					return nil, fmt.Errorf("save llm triage result: %w", err)
				}
				return &TriageResultWrapper{Record: saved, Response: resp}, nil
			}
		}
	}

	record := e.buildRecord(msg, commonLabels, store.TriageDecisionEnrichOnly, 0, "", "", "Fallback: no triage rules matched and LLM unavailable", nil, "", start)
	saved, err := e.triageResultStore.Create(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("save fallback triage result: %w", err)
	}
	return &TriageResultWrapper{Record: saved, Response: &TriageResponse{Decision: store.TriageDecisionEnrichOnly}}, nil
}

func (e *Engine) gatherContext(ctx context.Context, msg rabbitmq.TriageMessage, commonLabels map[string]string) TriageInput {
	input := TriageInput{
		CorrelationKey: msg.CorrelationKey,
		Severity:       msg.Severity,
	}
	for _, a := range msg.Alerts {
		input.Alerts = append(input.Alerts, AlertSnapshot{
			Fingerprint:  a.Fingerprint,
			AlertName:    a.Labels["alertname"],
			Status:       a.Status,
			Labels:       a.Labels,
			Annotations:  a.Annotations,
			GeneratorURL: a.GeneratorURL,
		})
	}
	if e.episodicFinder != nil {
		input.EpisodicEntries = e.episodicFinder.Find(ctx, msg.CorrelationKey, e.cfg.TriageContextEpisodicLimit)
	}
	if e.notesFinder != nil {
		input.NotesEntries = e.notesFinder.Find(ctx, commonLabels, e.cfg.TriageContextNotesLimit)
	}
	if e.memoryFinder != nil {
		input.MemoryEntries = e.memoryFinder.Find(ctx, msg.CorrelationKey, commonLabels, e.cfg.TriageContextMemoriesLimit)
	}
	if e.concurrentFinder != nil {
		input.ConcurrentCount = e.concurrentFinder.Count(ctx, msg.CorrelationKey)
	}
	return input
}

func (e *Engine) buildRecord(msg rabbitmq.TriageMessage, commonLabels map[string]string, decision string, confidence float64, severity, category, reasoning string, suggestedActions []string, modelUsed string, start time.Time) *store.TriageResultRecord {
	fps := make([]string, len(msg.Alerts))
	for i, a := range msg.Alerts {
		fps[i] = a.Fingerprint
	}
	if commonLabels == nil {
		commonLabels = map[string]string{}
	}
	if suggestedActions == nil {
		suggestedActions = []string{}
	}
	return &store.TriageResultRecord{
		CorrelationKey:     msg.CorrelationKey,
		AlertCount:         len(msg.Alerts),
		AlertFingerprints:  fps,
		AlertLabels:        commonLabels,
		SeverityInput:      msg.Severity,
		Decision:           decision,
		Confidence:         confidence,
		SeverityClassified: severity,
		Category:           category,
		Reasoning:          reasoning,
		SuggestedActions:   suggestedActions,
		Enrichment:         map[string]any{},
		ContextUsed:        map[string]any{},
		Outcome:            store.TriageResultOutcomePending,
		ModelUsed:          modelUsed,
		TriageDurationMs:   time.Since(start).Milliseconds(),
		TraceID:            msg.TraceID,
	}
}
