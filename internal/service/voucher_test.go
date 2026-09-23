package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestIsRedisUnavailable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "空错误", err: nil, want: false},
		{name: "超时", err: context.DeadlineExceeded, want: true},
		{name: "网络超时", err: timeoutError{}, want: true},
		{name: "连接拒绝", err: errors.New("dial tcp 127.0.0.1:6379: connection refused"), want: true},
		{name: "连接重置", err: errors.New("read: connection reset by peer"), want: true},
		{name: "业务错误", err: ErrAlreadyPurchased, want: false},
		{name: "取消不是故障", err: context.Canceled, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRedisUnavailable(tt.err); got != tt.want {
				t.Fatalf("isRedisUnavailable() = %v, want %v", got, tt.want)
			}
		})
	}

	var netErr net.Error = timeoutError{}
	if !isRedisUnavailable(netErr) {
		t.Fatal("net.Error 超时应视为 Redis 不可用")
	}
}

func TestIsMySQLUnavailable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "空错误", err: nil, want: false},
		{name: "超时", err: context.DeadlineExceeded, want: true},
		{name: "死锁", err: &mysqlDriver.MySQLError{Number: 1213, Message: "Deadlock found"}, want: true},
		{name: "连接丢失", err: &mysqlDriver.MySQLError{Number: 2013, Message: "Lost connection"}, want: true},
		{name: "锁等待", err: &mysqlDriver.MySQLError{Number: 1205, Message: "Lock wait timeout"}, want: true},
		{name: "重复键不是故障", err: &mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry"}, want: false},
		{name: "库存不足不是故障", err: errors.New("秒杀券库存不足"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMySQLUnavailable(tt.err); got != tt.want {
				t.Fatalf("isMySQLUnavailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleSeckillMessageRejectsPoison(t *testing.T) {
	s := &VoucherService{}
	err := s.handleSeckillMessage(context.Background(), redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"orderId": "bad"},
	})
	if !errors.Is(err, errPoisonSeckillMsg) {
		t.Fatalf("无效消息应视为毒消息，得到 %v", err)
	}
}

func TestSleepWithContextReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	sleepWithContext(ctx, time.Second)
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("已取消的 context 不应继续等待退避")
	}
}
