package rabbitmq

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestAMQPCarrierRoundTrip(t *testing.T) {
	c := NewAMQPCarrier(nil)
	if c == nil {
		t.Fatal("nil carrier")
	}
	c.Set("traceparent", "tp-value")
	if got := c.Get("traceparent"); got != "tp-value" {
		t.Errorf("Get = %q, want tp-value", got)
	}
	if c.Get("missing") != "" {
		t.Errorf("Get(missing) should be empty, got %q", c.Get("missing"))
	}
	keys := c.Keys()
	if len(keys) != 1 || keys[0] != "traceparent" {
		t.Errorf("Keys = %v", keys)
	}
	// Carrier must be usable as message headers.
	headers := amqp.Table(*c)
	if headers["traceparent"] != "tp-value" {
		t.Errorf("headers not propagated: %v", headers)
	}
}
