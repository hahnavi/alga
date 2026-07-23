package rabbitmq

import (
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/logger"
)

// Client wraps an AMQP connection with auto-reconnection support.
type Client struct {
	uri          string
	conn         *amqp.Connection
	mu           sync.RWMutex
	stopOnce     sync.Once
	stopCh       chan struct{}
	onReconnects []func()
	reconMu      sync.Mutex
}

func (c *Client) addOnReconnect(fn func()) {
	c.reconMu.Lock()
	c.onReconnects = append(c.onReconnects, fn)
	c.reconMu.Unlock()
}

// NewClient creates a new RabbitMQ client. Returns nil if uri is empty.
func NewClient(uri string) (*Client, error) {
	if uri == "" {
		return nil, nil
	}

	c := &Client{
		uri:    uri,
		stopCh: make(chan struct{}),
	}

	if err := c.connect(); err != nil {
		return nil, err
	}

	go c.reconnect()
	return c, nil
}

func (c *Client) connect() error {
	conn, err := amqp.Dial(c.uri)
	if err != nil {
		logger.Warn("failed to connect to RabbitMQ", "component", "rabbitmq", "error", err)
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	logger.Info("connected to RabbitMQ", "component", "rabbitmq")
	return nil
}

func (c *Client) reconnect() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("rabbitmq-reconnect panic recovered", "panic", r)
		}
	}()
	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-c.stopCh:
			return
		case <-closeCh:
		}

		// Attempt reconnection with backoff
		for attempt := 0; ; attempt++ {
			select {
			case <-c.stopCh:
				return
			default:
			}

			if err := c.connect(); err == nil {
				logger.Info("reconnected to RabbitMQ", "component", "rabbitmq", "attempt", attempt+1)
				c.reconMu.Lock()
				callbacks := make([]func(), len(c.onReconnects))
				copy(callbacks, c.onReconnects)
				c.reconMu.Unlock()
				for _, fn := range callbacks {
					fn()
				}
				break
			}

			delay := time.Duration(attempt+1) * 2 * time.Second
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			logger.Warn("RabbitMQ reconnection attempt failed, retrying", "component", "rabbitmq", "attempt", attempt+1, "delay", delay)
			timer := time.NewTimer(delay)
			select {
			case <-c.stopCh:
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

// Channel creates a new AMQP channel.
func (c *Client) Channel() (*amqp.Channel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return nil, errors.New("rabbitmq connection not established")
	}
	return c.conn.Channel()
}

// Close gracefully shuts down the RabbitMQ connection.
func (c *Client) Close() error {
	logger.Debug("closing RabbitMQ connection", "component", "rabbitmq")
	c.stopOnce.Do(func() { close(c.stopCh) })
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
