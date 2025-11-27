package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/nats-io/nats.go"
)

type Subscriber struct {
	conn   *nats.Conn
	sub    *nats.Subscription
	logger domain.Logger
}

func NewSubscriber(natsURL string, logger domain.Logger) (*Subscriber, error) {
	opts := []nats.Option{
		nats.ReconnectWait(time.Second * 2),
		nats.MaxReconnects(-1),
		nats.ReconnectBufSize(5 * 1024 * 1024),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected", map[string]interface{}{
				"server": nc.ConnectedUrl(),
			})
		}),
		nats.DisconnectHandler(func(nc *nats.Conn) {
			logger.Error("NATS disconnected - attempting reconnection", nc.LastError(), map[string]interface{}{
				"server": nc.ConnectedUrl(),
			})
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Error("NATS connection permanently closed", nc.LastError(), map[string]interface{}{
				"server": nc.ConnectedUrl(),
			})
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			logger.Error("NATS error", err, map[string]interface{}{
				"subject": sub.Subject,
			})
		}),
	}

	conn, err := nats.Connect(natsURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS: %w", err)
	}

	logger.Info("Connected to NATS", map[string]interface{}{
		"server": conn.ConnectedUrl(),
	})

	return &Subscriber{
		conn:   conn,
		logger: logger,
	}, nil
}

func (s *Subscriber) Subscribe(ctx context.Context, subject string, handler func(*domain.MeetingAlert)) error {
	messageHandler := func(msg *nats.Msg) {
		var alert domain.MeetingAlert
		if err := json.Unmarshal(msg.Data, &alert); err != nil {
			s.logger.Error("Failed to unmarshal message", err, map[string]interface{}{
				"subject": msg.Subject,
				"data":    string(msg.Data),
			})
			return
		}

		s.logger.Info("Received meeting alert", map[string]interface{}{
			"subject": msg.Subject,
			"title":   alert.Title,
			"when":    alert.When,
			"lead":    alert.Lead,
			"uid":     alert.EventUID(),
		})

		handler(&alert)
	}

	sub, err := s.conn.Subscribe(subject, messageHandler)
	if err != nil {
		return fmt.Errorf("subscribing to subject %s: %w", subject, err)
	}

	s.sub = sub
	s.logger.Info("Subscribed to NATS subject", map[string]interface{}{
		"subject": subject,
	})

	// Wait for context cancellation
	<-ctx.Done()
	return ctx.Err()
}

func (s *Subscriber) Close() error {
	if s.sub != nil {
		if err := s.sub.Unsubscribe(); err != nil {
			s.logger.Error("Failed to unsubscribe", err, nil)
		}
	}

	if s.conn != nil {
		s.conn.Close()
		s.logger.Info("NATS connection closed", nil)
	}

	return nil
}

// Health returns the connection status
func (s *Subscriber) Health() map[string]interface{} {
	if s.conn == nil {
		return map[string]interface{}{
			"connected": false,
			"status":    "not_initialized",
		}
	}

	status := s.conn.Status()
	return map[string]interface{}{
		"connected":  s.conn.IsConnected(),
		"status":     status.String(),
		"server":     s.conn.ConnectedUrl(),
		"reconnects": s.conn.Stats().Reconnects,
		"last_error": s.conn.LastError(),
	}
}

// GetConnection returns the underlying NATS connection
func (s *Subscriber) GetConnection() *nats.Conn {
	return s.conn
}

// IsConnected returns true if the NATS connection is active
func (s *Subscriber) IsConnected() bool {
	return s.conn != nil && s.conn.IsConnected()
}
