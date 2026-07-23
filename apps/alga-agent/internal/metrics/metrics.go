// Package metrics implements an in-memory counter store with a hand-written
// Prometheus text-format exposition endpoint (SPEC §10). Zero external deps.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing counter.
type Counter struct {
	name   string
	help   string
	labels []string
	mu     sync.Mutex
	values map[string]uint64 // key = joined label values
}

// NewCounter constructs a counter with the given label names.
func NewCounter(name, help string, labels ...string) *Counter {
	return &Counter{name: name, help: help, labels: labels, values: make(map[string]uint64)}
}

// Inc increments the counter for the given label values by 1.
func (c *Counter) Inc(labelValues ...string) {
	c.IncBy(1, labelValues...)
}

// IncBy increments the counter for the given label values by n.
func (c *Counter) IncBy(n uint64, labelValues ...string) {
	if len(labelValues) != len(c.labels) {
		// Mismatched arity; ignore to avoid panics in hot paths.
		return
	}
	key := strings.Join(labelValues, "\x00")
	c.mu.Lock()
	c.values[key] += n
	c.mu.Unlock()
}

// WritePrometheus writes the counter in Prometheus text format to w.
func (c *Counter) WritePrometheus(w io.Writer) {
	if len(c.values) == 0 {
		return
	}
	if c.help != "" {
		fmt.Fprintf(w, "# HELP %s %s\n", c.name, c.help)
	}
	fmt.Fprintf(w, "# TYPE %s counter\n", c.name)
	type entry struct {
		key   string
		value uint64
	}
	entries := make([]entry, 0, len(c.values))
	c.mu.Lock()
	for k, v := range c.values {
		entries = append(entries, entry{k, v})
	}
	c.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	for _, e := range entries {
		labelVals := strings.Split(e.key, "\x00")
		if len(c.labels) == 0 {
			fmt.Fprintf(w, "%s %d\n", c.name, e.value)
			continue
		}
		parts := make([]string, len(c.labels))
		for i, lv := range labelVals {
			parts[i] = fmt.Sprintf("%s=%q", c.labels[i], lv)
		}
		fmt.Fprintf(w, "%s{%s} %d\n", c.name, strings.Join(parts, ","), e.value)
	}
}

// Gauge is a value that can go up or down.
type Gauge struct {
	name  string
	help  string
	value atomic.Uint64
}

// NewGauge constructs a gauge.
func NewGauge(name, help string) *Gauge { return &Gauge{name: name, help: help} }

// Set sets the gauge value.
func (g *Gauge) Set(v uint64) { g.value.Store(v) }

// WritePrometheus writes the gauge in Prometheus text format.
func (g *Gauge) WritePrometheus(w io.Writer) {
	if g.help != "" {
		fmt.Fprintf(w, "# HELP %s %s\n", g.name, g.help)
	}
	fmt.Fprintf(w, "# TYPE %s gauge\n", g.name)
	fmt.Fprintf(w, "%s %d\n", g.name, g.value.Load())
}

// Registry is a collection of metrics.
type Registry struct {
	mu       sync.Mutex
	counters []*Counter
	gauges   []*Gauge
}

// New returns an empty Registry.
func New() *Registry { return &Registry{} }

// Register adds a counter.
func (r *Registry) Register(c *Counter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters = append(r.counters, c)
}

// RegisterGauge adds a gauge.
func (r *Registry) RegisterGauge(g *Gauge) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges = append(r.gauges, g)
}

// WritePrometheus writes all metrics in Prometheus text format.
func (r *Registry) WritePrometheus(w io.Writer) {
	r.mu.Lock()
	counters := append([]*Counter(nil), r.counters...)
	gauges := append([]*Gauge(nil), r.gauges...)
	r.mu.Unlock()

	sort.Slice(counters, func(i, j int) bool { return counters[i].name < counters[j].name })
	sort.Slice(gauges, func(i, j int) bool { return gauges[i].name < gauges[j].name })

	for _, c := range counters {
		c.WritePrometheus(w)
	}
	for _, g := range gauges {
		g.WritePrometheus(w)
	}
}

// HTTPHandler returns an http.HandlerFunc that exposes all metrics at
// /metrics in Prometheus text format.
func (r *Registry) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.WritePrometheus(w)
	}
}

// Standard metric names used across the agent.
type Metrics struct {
	Registry *Registry

	// Messages received, by channel.
	MessagesReceived *Counter
	// Messages processed successfully, by channel.
	MessagesProcessed *Counter
	// Messages failed, by channel.
	MessagesFailed *Counter
	// Tool invocations, by tool name and status.
	ToolInvocations *Counter
	// LLM iterations across all sessions.
	LLMIterations *Counter
	// Active sessions.
	ActiveSessions *Gauge
}

// NewStandard returns a Metrics populated with the standard agent counters.
func NewStandard() *Metrics {
	r := New()
	m := &Metrics{
		Registry:          r,
		MessagesReceived:  NewCounter("alga_agent_messages_received_total", "Total messages received by channel", "channel"),
		MessagesProcessed: NewCounter("alga_agent_messages_processed_total", "Total messages processed successfully by channel", "channel"),
		MessagesFailed:    NewCounter("alga_agent_messages_failed_total", "Total messages that failed processing by channel", "channel"),
		ToolInvocations:   NewCounter("alga_agent_tool_invocations_total", "Tool invocations by name and status", "tool", "status"),
		LLMIterations:     NewCounter("alga_agent_llm_iterations_total", "Total LLM iterations across all sessions"),
		ActiveSessions:    NewGauge("alga_agent_active_sessions", "Number of active sessions"),
	}
	r.Register(m.MessagesReceived)
	r.Register(m.MessagesProcessed)
	r.Register(m.MessagesFailed)
	r.Register(m.ToolInvocations)
	r.Register(m.LLMIterations)
	r.RegisterGauge(m.ActiveSessions)
	return m
}
