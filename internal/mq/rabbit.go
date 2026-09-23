package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	Exchange        = "dianping.seckill"
	OrderQueue      = "seckill.order"
	RetryQueue      = "seckill.order.retry"
	DeadLetterQueue = "seckill.order.dead"
	RoutingKey      = "order"
)

type OrderMessage struct {
	OrderID   int64 `json:"orderId"`
	UserID    int64 `json:"userId"`
	VoucherID int64 `json:"voucherId"`
}

type Client struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func Dial(url string) (*Client, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, ch: ch}
	if err := c.declare(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) declare() error {
	if err := c.ch.ExchangeDeclare(Exchange, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := c.ch.QueueDeclare(DeadLetterQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := c.ch.QueueBind(DeadLetterQueue, "dead", Exchange, false, nil); err != nil {
		return err
	}
	retryArgs := amqp.Table{"x-message-ttl": int32(1000), "x-dead-letter-exchange": Exchange, "x-dead-letter-routing-key": RoutingKey}
	if _, err := c.ch.QueueDeclare(RetryQueue, true, false, false, false, retryArgs); err != nil {
		return err
	}
	if err := c.ch.QueueBind(RetryQueue, "retry", Exchange, false, nil); err != nil {
		return err
	}
	args := amqp.Table{"x-dead-letter-exchange": Exchange, "x-dead-letter-routing-key": "retry"}
	if _, err := c.ch.QueueDeclare(OrderQueue, true, false, false, false, args); err != nil {
		return err
	}
	return c.ch.QueueBind(OrderQueue, RoutingKey, Exchange, false, nil)
}

func (c *Client) Publish(ctx context.Context, msg OrderMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if ok {
		_ = c.ch.Qos(1, 0, false)
		_ = deadline
	}
	return c.ch.PublishWithContext(ctx, Exchange, RoutingKey, false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body, Timestamp: time.Now()})
}

func (c *Client) Consume(ctx context.Context, fn func(context.Context, OrderMessage) error) error {
	d, err := c.ch.Consume(OrderQueue, "seckill-worker", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-d:
			if !ok {
				return fmt.Errorf("rabbitmq consumer closed")
			}
			var msg OrderMessage
			if err := json.Unmarshal(delivery.Body, &msg); err != nil {
				_ = delivery.Reject(false)
				continue
			}
			err := fn(ctx, msg)
			if err == nil {
				_ = delivery.Ack(false)
				continue
			}
			attempts := intHeader(delivery.Headers, "x-retry-count")
			if attempts >= 5 || !isRetryable(err) {
				_ = c.ch.PublishWithContext(ctx, Exchange, "dead", false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: delivery.Body, Headers: amqp.Table{"reason": err.Error(), "attempts": attempts}})
			} else {
				_ = c.ch.PublishWithContext(ctx, Exchange, "retry", false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: delivery.Body, Headers: amqp.Table{"x-retry-count": attempts + 1}})
			}
			_ = delivery.Ack(false)
		}
	}
}

func intHeader(headers amqp.Table, key string) int {
	if v, ok := headers[key].(int32); ok {
		return int(v)
	}
	if v, ok := headers[key].(int); ok {
		return v
	}
	return 0
}

func isRetryable(err error) bool {
	return err != nil && (err == context.DeadlineExceeded || err == context.Canceled)
}
func (c *Client) Close() {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}
