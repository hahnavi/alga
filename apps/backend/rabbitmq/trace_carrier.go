package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPCarrier adapts an amqp.Table to the OpenTelemetry
// propagation.TextMapCarrier interface so W3C trace context can ride along in
// message headers across the broker boundary.
type AMQPCarrier amqp.Table

// NewAMQPCarrier wraps an existing amqp.Table as a trace carrier. If table is
// nil a fresh one is allocated (so Set is always safe).
func NewAMQPCarrier(table amqp.Table) *AMQPCarrier {
	if table == nil {
		table = amqp.Table{}
	}
	c := AMQPCarrier(table)
	return &c
}

func (c *AMQPCarrier) Get(key string) string {
	v, ok := (*c)[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func (c *AMQPCarrier) Set(key, value string) {
	(*c)[key] = value
}

func (c *AMQPCarrier) Keys() []string {
	keys := make([]string, 0, len(*c))
	for k := range *c {
		keys = append(keys, k)
	}
	return keys
}
