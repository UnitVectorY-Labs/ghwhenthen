package consumer

import (
	"context"
	"log/slog"

	"cloud.google.com/go/pubsub"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/health"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/rule"
)

// PubSubClient is an interface wrapping the Pub/Sub client for testability.
type PubSubClient interface {
	Subscription(id string) PubSubSubscription
	Close() error
}

// PubSubSubscription is an interface wrapping the Pub/Sub subscription for testability.
type PubSubSubscription interface {
	Receive(ctx context.Context, f func(ctx context.Context, msg PubSubMessage)) error
}

// PubSubMessage is an interface wrapping a Pub/Sub message for testability.
type PubSubMessage interface {
	ID() string
	Data() []byte
	Attributes() map[string]string
	Ack()
	Nack()
}

// Consumer connects to Pub/Sub and processes messages through the rule engine.
type Consumer struct {
	projectID      string
	subscriptionID string
	onFailure      string // "ack" or "nack"
	engine         *rule.Engine
	health         *health.Status
	logger         *slog.Logger

	clientFactory func(ctx context.Context, projectID string) (PubSubClient, error)
}

// Option configures a Consumer.
type Option func(*Consumer)

// WithClientFactory overrides the default Pub/Sub client factory.
func WithClientFactory(factory func(ctx context.Context, projectID string) (PubSubClient, error)) Option {
	return func(c *Consumer) {
		c.clientFactory = factory
	}
}

// WithLogger overrides the default logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Consumer) {
		c.logger = logger
	}
}

// New creates a Consumer with the given configuration and options.
func New(projectID, subscriptionID, onFailure string, engine *rule.Engine, health *health.Status, opts ...Option) *Consumer {
	c := &Consumer{
		projectID:      projectID,
		subscriptionID: subscriptionID,
		onFailure:      onFailure,
		engine:         engine,
		health:         health,
		logger:         slog.Default(),
		clientFactory:  defaultClientFactory,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run connects to Pub/Sub and processes messages until the context is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	client, err := c.clientFactory(ctx, c.projectID)
	if err != nil {
		return err
	}
	defer client.Close()

	sub := client.Subscription(c.subscriptionID)
	c.health.SetReady(true)

	err = sub.Receive(ctx, func(ctx context.Context, msg PubSubMessage) {
		c.handleMessage(ctx, msg)
	})

	c.health.SetReady(false)
	return err
}

func (c *Consumer) handleMessage(ctx context.Context, msg PubSubMessage) {
	attrs := msg.Attributes()

	c.logger.InfoContext(ctx, "received message",
		"message_id", msg.ID(),
		"gh_delivery", attrs["gh_delivery"],
		"gh_event", attrs["gh_event"],
		"action", attrs["action"],
		"repository", attrs["repository"],
	)

	evt, err := event.BuildEvent(msg.Data(), attrs)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to build event",
			"message_id", msg.ID(),
			"error", err,
		)
		c.handleResult(msg, err)
		return
	}

	matched, ruleName, err := c.engine.ProcessEvent(ctx, evt)
	if err != nil {
		c.logger.ErrorContext(ctx, "rule execution failed",
			"message_id", msg.ID(),
			"rule_name", ruleName,
			"error", err,
		)
		c.handleResult(msg, err)
		return
	}

	if !matched {
		c.logger.InfoContext(ctx, "no matching rule",
			"message_id", msg.ID(),
		)
		msg.Ack()
		return
	}

	c.logger.InfoContext(ctx, "rule executed successfully",
		"message_id", msg.ID(),
		"rule_name", ruleName,
	)
	msg.Ack()
}

func (c *Consumer) handleResult(msg PubSubMessage, err error) {
	if err == nil {
		msg.Ack()
		return
	}
	if c.onFailure == "nack" {
		msg.Nack()
	} else {
		msg.Ack()
	}
}

// defaultClientFactory creates a real Google Cloud Pub/Sub client.
func defaultClientFactory(ctx context.Context, projectID string) (PubSubClient, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &realClient{client: client}, nil
}

// realClient wraps *pubsub.Client to implement PubSubClient.
type realClient struct {
	client *pubsub.Client
}

func (rc *realClient) Subscription(id string) PubSubSubscription {
	sub := rc.client.Subscription(id)
	sub.ReceiveSettings.MaxOutstandingMessages = 1
	sub.ReceiveSettings.NumGoroutines = 1
	return &realSubscription{sub: sub}
}

func (rc *realClient) Close() error {
	return rc.client.Close()
}

// realSubscription wraps *pubsub.Subscription to implement PubSubSubscription.
type realSubscription struct {
	sub *pubsub.Subscription
}

func (rs *realSubscription) Receive(ctx context.Context, f func(ctx context.Context, msg PubSubMessage)) error {
	return rs.sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		f(ctx, &realMessage{msg: msg})
	})
}

// realMessage wraps *pubsub.Message to implement PubSubMessage.
type realMessage struct {
	msg *pubsub.Message
}

func (rm *realMessage) ID() string                  { return rm.msg.ID }
func (rm *realMessage) Data() []byte                { return rm.msg.Data }
func (rm *realMessage) Attributes() map[string]string { return rm.msg.Attributes }
func (rm *realMessage) Ack()                         { rm.msg.Ack() }
func (rm *realMessage) Nack()                        { rm.msg.Nack() }
