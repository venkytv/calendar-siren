package heartbeat

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meeting-siren/meeting-siren/internal/domain"
	"github.com/nats-io/nats.go"
	"github.com/venkytv/nats-heartbeat/pkg/heartbeat"
)

type Publisher struct {
	nc          *nats.Conn
	config      *domain.Config
	logger      domain.Logger
	publisher   *heartbeat.Publisher
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stopOnce    sync.Once
	startedOnce sync.Once
}

func NewPublisher(nc *nats.Conn, config *domain.Config, logger domain.Logger) *Publisher {
	return &Publisher{
		nc:     nc,
		config: config,
		logger: logger,
	}
}

func (p *Publisher) Start(ctx context.Context) error {
	var startErr error
	p.startedOnce.Do(func() {
		if !p.config.HeartbeatEnabled {
			p.logger.Info("heartbeat disabled, skipping", nil)
			return
		}

		if p.config.HeartbeatSubject == "" {
			startErr = fmt.Errorf("heartbeat subject is required when heartbeat is enabled")
			return
		}

		if p.config.HeartbeatInterval <= 0 {
			startErr = fmt.Errorf("heartbeat interval must be positive")
			return
		}

		p.ctx, p.cancel = context.WithCancel(ctx)
		p.publisher = heartbeat.NewPublisher(p.nc, "")

		p.logger.Info("starting heartbeat publisher", map[string]interface{}{
			"subject":      p.config.HeartbeatSubject,
			"interval":     p.config.HeartbeatInterval,
			"description":  p.config.HeartbeatDescription,
			"grace_period": p.config.HeartbeatGracePeriod,
		})

		p.wg.Add(1)
		go p.publishLoop()
	})

	return startErr
}

func (p *Publisher) publishLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(time.Duration(p.config.HeartbeatInterval) * time.Second)
	defer ticker.Stop()

	// Publish immediately on start
	p.publishHeartbeat()

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Info("heartbeat publisher stopping", nil)
			return
		case <-ticker.C:
			p.publishHeartbeat()
		}
	}
}

func (p *Publisher) publishHeartbeat() {
	msg := heartbeat.Message{
		Subject:     p.config.HeartbeatSubject,
		GeneratedAt: time.Now().UTC(),
		Interval:    time.Duration(p.config.HeartbeatInterval) * time.Second,
		Description: p.config.HeartbeatDescription,
	}

	if p.config.HeartbeatGracePeriod > 0 {
		gracePeriod := time.Duration(p.config.HeartbeatGracePeriod) * time.Second
		msg.GracePeriod = &gracePeriod
	}

	if err := p.publisher.Publish(p.ctx, msg); err != nil {
		p.logger.Error("failed to publish heartbeat", err, map[string]interface{}{
			"subject": p.config.HeartbeatSubject,
		})
		return
	}

	p.logger.Debug("heartbeat published", map[string]interface{}{
		"subject":   p.config.HeartbeatSubject,
		"timestamp": msg.GeneratedAt,
	})
}

func (p *Publisher) Stop() error {
	var stopErr error
	p.stopOnce.Do(func() {
		if p.cancel != nil {
			p.cancel()
			p.wg.Wait()
			p.logger.Info("heartbeat publisher stopped", nil)
		}
	})
	return stopErr
}
