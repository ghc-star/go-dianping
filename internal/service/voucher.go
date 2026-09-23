package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/learning/go-dianping/internal/model"
	"github.com/learning/go-dianping/internal/mq"
	"github.com/learning/go-dianping/internal/redisx"
	"github.com/learning/go-dianping/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type VoucherService struct {
	repo   *repository.Repository
	rdb    *redis.Client
	rabbit *mq.Client
}

func (s *VoucherService) ConfigureRabbit(client *mq.Client) { s.rabbit = client }

func NewVoucherService(
	repo *repository.Repository,
	rdb *redis.Client,
) *VoucherService {
	return &VoucherService{
		repo: repo,
		rdb:  rdb,
	}
}

func (s *VoucherService) Create(
	ctx context.Context,
	voucher *model.Voucher,
) (int64, error) {
	if err := s.validateVoucher(ctx, voucher); err != nil {
		return 0, err
	}

	// 以下字段由服务端决定，不能接受客户端指定的值。
	voucher.ID = 0
	voucher.Type = 0
	voucher.Status = 1
	voucher.CreateTime = time.Time{}
	voucher.UpdateTime = time.Time{}
	voucher.Stock = nil
	voucher.BeginTime = nil
	voucher.EndTime = nil

	if err := s.repo.VoucherCreate(ctx, voucher); err != nil {
		return 0, fmt.Errorf("创建优惠券：%w", err)
	}

	return voucher.ID, nil
}

func (s *VoucherService) ListByShop(
	ctx context.Context,
	shopID int64,
) ([]model.Voucher, error) {
	if shopID <= 0 {
		return nil, fmt.Errorf(
			"%w：商铺ID必须大于0",
			ErrInvalidVoucher,
		)
	}

	vouchers, err := s.repo.VoucherListByShop(ctx, shopID)

	if err != nil {
		return nil, fmt.Errorf("查询优惠卷:%w", err)
	}
	return vouchers, nil
}

func (s *VoucherService) validateVoucher(
	ctx context.Context,
	voucher *model.Voucher,
) error {
	if voucher == nil {
		return ErrInvalidVoucher
	}

	voucher.Title = strings.TrimSpace(voucher.Title)

	if voucher.ShopID <= 0 {
		return fmt.Errorf("%w:商铺ID必须大于0", ErrInvalidVoucher)
	}
	if voucher.Title == "" ||
		utf8.RuneCountInString(voucher.Title) > 255 {
		return fmt.Errorf(
			"%w:标题必填且最长255字",
			ErrInvalidVoucher,
		)
	}
	if utf8.RuneCountInString(voucher.SubTitle) > 255 {
		return fmt.Errorf("%w：副标题最长255字", ErrInvalidVoucher)
	}
	if utf8.RuneCountInString(voucher.Rules) > 1024 {
		return fmt.Errorf("%w：使用规则最长1024字", ErrInvalidVoucher)
	}
	if voucher.PayValue <= 0 || voucher.ActualValue <= 0 {
		return fmt.Errorf("%w：金额必须大于0", ErrInvalidVoucher)
	}
	if voucher.PayValue > voucher.ActualValue {
		return fmt.Errorf(
			"%w：支付金额不能大于抵扣金额",
			ErrInvalidVoucher,
		)
	}

	_, err := s.repo.FindById(ctx, voucher.ShopID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w：商铺不存在", ErrInvalidVoucher)
	}
	if err != nil {
		return fmt.Errorf("查询商铺：%w", err)
	}

	return nil
}

func (s *VoucherService) CreateSeckill(
	ctx context.Context,
	voucher *model.Voucher,
	campaign *model.SeckillVoucher,
) (int64, error) {
	if err := s.validateVoucher(ctx, voucher); err != nil {
		return 0, err
	}
	if campaign == nil {
		return 0, ErrInvalidVoucher
	}
	if campaign.Stock <= 0 {
		return 0, fmt.Errorf(
			"%w：秒杀库存必须大于0",
			ErrInvalidVoucher,
		)
	}
	if campaign.BeginTime.IsZero() ||
		campaign.EndTime.IsZero() {
		return 0, fmt.Errorf(
			"%w：秒杀开始和结束时间必填",
			ErrInvalidVoucher,
		)
	}
	if !campaign.EndTime.After(campaign.BeginTime) {
		return 0, fmt.Errorf(
			"%w：结束时间必须晚于开始时间",
			ErrInvalidVoucher,
		)
	}

	voucher.ID = 0
	voucher.Type = 1
	voucher.Status = 1
	voucher.CreateTime = time.Time{}
	voucher.UpdateTime = time.Time{}
	voucher.Stock = nil
	voucher.BeginTime = nil
	voucher.EndTime = nil

	campaign.VoucherID = 0
	campaign.CreateTime = time.Time{}
	campaign.UpdateTime = time.Time{}

	if err := s.repo.SeckillVoucherCreate(
		ctx,
		voucher,
		campaign,
	); err != nil {
		return 0, fmt.Errorf("创建秒杀券：%w", err)
	}

	if err := s.initSeckillRedis(ctx, campaign); err != nil {
		return voucher.ID, fmt.Errorf(
			"秒杀券已写入数据库，但 Redis 初始化失败：%w",
			err,
		)
	}

	return voucher.ID, nil
}

var (
	ErrInvalidVoucher       = errors.New("优惠券参数错误")
	ErrSeckillNotStarted    = errors.New("秒杀尚未开始")
	ErrSeckillEnded         = errors.New("秒杀已经结束")
	ErrSeckillOutOfStock    = errors.New("秒杀券库存不足")
	ErrAlreadyPurchased     = errors.New("不能重复购买")
	ErrVoucherOrderNotFound = errors.New("订单不存在")
)

func (s *VoucherService) SeckillSync(
	ctx context.Context,
	voucherID int64,
	userID int64,
) (int64, error) {
	if voucherID <= 0 {
		return 0, fmt.Errorf(
			"%w：优惠券ID必须大于0",
			ErrInvalidVoucher,
		)
	}
	if userID <= 0 {
		return 0, fmt.Errorf(
			"%w：用户ID无效",
			ErrInvalidVoucher,
		)
	}

	campaign, err := s.repo.FindSeckillVoucher(
		ctx,
		voucherID,
	)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf(
			"%w：秒杀券不存在",
			ErrInvalidVoucher,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("查询秒杀券：%w", err)
	}

	now := time.Now()
	if now.Before(campaign.BeginTime) {
		return 0, ErrSeckillNotStarted
	}
	if !now.Before(campaign.EndTime) {
		return 0, ErrSeckillEnded
	}
	orderID, err := s.nextOrderID(ctx)
	if err != nil {
		return 0, err
	}

	order := &model.VoucherOrder{
		ID:        orderID,
		UserID:    userID,
		VoucherID: voucherID,
		PayType:   1,
		Status:    1,
	}

	err = s.repo.CreateVoucherOrder(ctx, order)
	switch {
	case errors.Is(err, repository.ErrVoucherPurchased):
		return 0, ErrAlreadyPurchased
	case errors.Is(err, repository.ErrVoucherOutOfStock):
		return 0, ErrSeckillOutOfStock
	case err != nil:
		return 0, fmt.Errorf("创建秒杀订单：%w", err)
	}

	return order.ID, nil
}

func (s *VoucherService) nextOrderID(
	ctx context.Context,
) (int64, error) {
	now := time.Now().UTC()
	key := "practice:voucher-order:id:" +
		now.Format("2006:01:02")

	sequence, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		if isRedisUnavailable(err) {
			return 0, fmt.Errorf("生产订单序号：%w", ErrRedisUnavailable)
		}
		return 0, fmt.Errorf("生产订单序号:%w", err)
	}

	if sequence == 1 {
		if err := s.rdb.Expire(
			ctx,
			key,
			72*time.Hour,
		).Err(); err != nil {
			return 0, fmt.Errorf("设置订单序号有效期:%w", err)
		}
	}
	const customEpoch int64 = 1640995200
	timestamp := now.Unix() - customEpoch
	if timestamp <= 0 || sequence > 0xffffffff {
		return 0, errors.New("订单ID生成范围异常")
	}

	return timestamp<<32 | sequence, nil
}

func (s *VoucherService) FindOrder(
	ctx context.Context,
	orderID int64,
	userID int64,
) (model.VoucherOrder, error) {
	if orderID <= 0 || userID <= 0 {
		return model.VoucherOrder{},
			fmt.Errorf(
				"%w：订单ID或用户ID无效",
				ErrInvalidVoucher,
			)
	}

	order, err := s.repo.FindVoucherOrder(
		ctx,
		orderID,
		userID,
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.VoucherOrder{}, ErrVoucherOrderNotFound
	}
	if err != nil {
		if isMySQLUnavailable(err) {
			return model.VoucherOrder{},
				fmt.Errorf("查询订单：%w", ErrMySQLUnavailable)
		}
		return model.VoucherOrder{},
			fmt.Errorf("查询订单：%w", err)
	}

	return order, nil
}

type OrderStatus struct {
	OrderID int64  `json:"orderId"`
	Status  string `json:"status"`
}

func (s *VoucherService) FindOrderStatus(
	ctx context.Context,
	orderID int64,
	userID int64,
) (OrderStatus, error) {
	if orderID <= 0 || userID <= 0 {
		return OrderStatus{}, fmt.Errorf(
			"%w:订单ID或用户ID无效",
			ErrInvalidVoucher,
		)
	}
	_, err := s.repo.FindVoucherOrder(ctx, orderID, userID)
	if err == nil {
		return OrderStatus{
			OrderID: orderID,
			Status:  "success",
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		if isMySQLUnavailable(err) {
			return OrderStatus{}, fmt.Errorf(
				"查询订单：%w",
				ErrMySQLUnavailable,
			)
		}
		return OrderStatus{}, fmt.Errorf("查询订单：%w", err)
	}

	owner, err := s.rdb.Get(
		ctx,
		redisx.Key(redisx.OrderStatusPrefix, orderID),
	).Result()

	if errors.Is(err, redis.Nil) {
		return OrderStatus{}, ErrVoucherOrderNotFound
	}
	if err != nil {
		if isRedisUnavailable(err) {
			return OrderStatus{}, fmt.Errorf(
				"查询秒杀受理状态：%w",
				ErrRedisUnavailable,
			)
		}
		return OrderStatus{}, fmt.Errorf(
			"查询秒杀受理状态：%w",
			err,
		)
	}
	user := strconv.FormatInt(userID, 10)
	switch owner {
	case user:
		return OrderStatus{
			OrderID: orderID,
			Status:  "pending",
		}, nil
	case "failed:" + user:
		return OrderStatus{
			OrderID: orderID,
			Status:  "failed",
		}, nil
	default:
		return OrderStatus{}, ErrVoucherOrderNotFound
	}
}

func (s *VoucherService) initSeckillRedis(
	ctx context.Context,
	campaign *model.SeckillVoucher,
) error {
	keys := redisx.SeckillKeys(campaign.VoucherID)
	result, err := redisx.InitSeckillScript.Run(
		ctx,
		s.rdb,
		keys,
		campaign.Stock,
		campaign.BeginTime.UnixMilli(),
		campaign.EndTime.UnixMilli(),
	).Int()
	if err != nil {
		return fmt.Errorf("初始化秒杀 Redis：%w", err)
	}
	if result != 0 {
		return errors.New("秒杀 Redis 数据已经存在，拒绝覆盖")
	}

	return nil
}

func (s *VoucherService) SeckillAsync(
	ctx context.Context,
	voucherID int64,
	userID int64,
) (int64, error) {
	if voucherID <= 0 {
		return 0, fmt.Errorf(
			"%w:优惠劵ID必须大于0",
			ErrInvalidVoucher,
		)
	}

	if userID <= 0 {
		return 0, fmt.Errorf(
			"%w:用户ID无效",
			ErrInvalidVoucher,
		)
	}
	orderID, err := s.nextOrderID(ctx)
	if err != nil {
		return 0, err
	}

	result, err := redisx.SeckillScript.Run(
		ctx,
		s.rdb,
		redisx.SeckillOrderKeys(voucherID, orderID),
		time.Now().UnixMilli(),
		userID,
		voucherID,
		orderID,
	).Int()

	if err != nil {
		if acceptedID, recovered, recErr := s.recoverSeckillAcceptance(
			ctx,
			voucherID,
			userID,
		); recErr == nil && recovered {
			return acceptedID, nil
		}
		if isRedisUnavailable(err) {
			return 0, fmt.Errorf("秒杀资格校验：%w", ErrRedisUnavailable)
		}
		return 0, fmt.Errorf("秒杀资格校验:%w", err)
	}

	switch result {
	case 1:
		if s.rabbit != nil {
			if err := s.rabbit.Publish(ctx, mq.OrderMessage{OrderID: orderID, UserID: userID, VoucherID: voucherID}); err != nil {
				// The Lua script already reserved the order. Return the reservation
				// to Redis so a broker outage cannot permanently consume inventory.
				_ = s.compensateSeckill(ctx, redis.XMessage{Values: map[string]interface{}{"orderId": orderID, "userId": userID, "voucherId": voucherID}})
				return 0, fmt.Errorf("发布秒杀订单：%w", err)
			}
		}
		return orderID, nil
	case -1:
		return 0, fmt.Errorf(
			"%w：秒杀券不存在",
			ErrInvalidVoucher,
		)
	case -2:
		return 0, ErrSeckillNotStarted
	case -3:
		return 0, ErrSeckillEnded
	case -4:
		return 0, ErrSeckillOutOfStock
	case -5:
		if acceptedID, recovered, recErr := s.recoverSeckillAcceptance(
			ctx,
			voucherID,
			userID,
		); recErr == nil && recovered {
			return acceptedID, nil
		}
		return 0, ErrAlreadyPurchased
	default:
		return 0, fmt.Errorf(
			"秒杀脚本返回未知结果：%d",
			result,
		)
	}
}

const (
	seckillConsumerGroup    = "cg:seckill"
	seckillIdleClaimAfter   = 30 * time.Second
	seckillReadBlock        = 2 * time.Second
	seckillMaxDeliveries    = 8
	seckillWorkerBackoff    = time.Second
	seckillWorkerBackoffMax = 8 * time.Second
)

var (
	ErrRedisUnavailable = errors.New("Redis暂时不可用")
	ErrMySQLUnavailable = errors.New("MySQL暂时不可用")
	errPoisonSeckillMsg = errors.New("秒杀消息无法处理")
)

func (s *VoucherService) recoverSeckillAcceptance(
	ctx context.Context,
	voucherID int64,
	userID int64,
) (int64, bool, error) {
	raw, err := s.rdb.HGet(
		ctx,
		redisx.Key(redisx.SeckillBuyersPrefix, voucherID),
		strconv.FormatInt(userID, 10),
	).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		if isRedisUnavailable(err) {
			return 0, false, ErrRedisUnavailable
		}
		return 0, false, err
	}

	acceptedID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || acceptedID <= 0 {
		return 0, false, nil
	}
	return acceptedID, true, nil
}

func seckillConsumerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("seckill-%s-%d", host, os.Getpid())
}

func isRedisUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "loading redis") ||
		strings.Contains(msg, "cluster down") ||
		strings.Contains(msg, "try again")
}

func isMySQLUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1205, 1213, 2006, 2013:
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid connection") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "deadlock") ||
		strings.Contains(msg, "try restarting transaction")
}

func sleepWithContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (s *VoucherService) ensureSeckillConsumerGroup(
	ctx context.Context,
) error {
	err := s.rdb.XGroupCreateMkStream(
		ctx,
		redisx.OrderStream,
		seckillConsumerGroup,
		"0",
	).Err()
	if err == nil ||
		strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	if isRedisUnavailable(err) {
		return fmt.Errorf("创建秒杀消费组：%w", ErrRedisUnavailable)
	}
	return fmt.Errorf("创建秒杀消费组：%w", err)
}

func (s *VoucherService) StartSeckillWorker(
	ctx context.Context,
) {
	if s.rabbit != nil {
		for ctx.Err() == nil {
			err := s.rabbit.Consume(ctx, func(ctx context.Context, msg mq.OrderMessage) error {
				return s.handleSeckillMessage(ctx, redis.XMessage{Values: map[string]interface{}{"orderId": msg.OrderID, "userId": msg.UserID, "voucherId": msg.VoucherID}})
			})
			if ctx.Err() != nil {
				return
			}
			log.Printf("RabbitMQ秒杀Worker消费失败：%v", err)
			sleepWithContext(ctx, seckillWorkerBackoff)
		}
		return
	}
	consumer := seckillConsumerName()
	backoff := seckillWorkerBackoff

	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := s.ensureSeckillConsumerGroup(ctx); err != nil {
			log.Printf("秒杀Worker准备消费组失败：%v", err)
			sleepWithContext(ctx, backoff)
			if backoff < seckillWorkerBackoffMax {
				backoff *= 2
			}
			continue
		}

		err := s.consumeSeckillBatch(ctx, consumer)
		if err == nil {
			backoff = seckillWorkerBackoff
			continue
		}
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			if ctx.Err() != nil {
				return
			}
		}
		log.Printf("秒杀Worker消费失败：%v", err)
		sleepWithContext(ctx, backoff)
		if backoff < seckillWorkerBackoffMax {
			backoff *= 2
		}
	}
}

func (s *VoucherService) consumeSeckillBatch(
	ctx context.Context,
	consumer string,
) error {
	if err := s.claimIdleSeckillMessages(ctx, consumer); err != nil {
		return err
	}
	return s.readSeckillMessages(
		ctx,
		consumer,
		">",
		seckillReadBlock,
	)
}

func (s *VoucherService) claimIdleSeckillMessages(
	ctx context.Context,
	consumer string,
) error {
	start := "0-0"
	for {
		messages, next, err := s.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   redisx.OrderStream,
			Group:    seckillConsumerGroup,
			Consumer: consumer,
			MinIdle:  seckillIdleClaimAfter,
			Start:    start,
			Count:    10,
		}).Result()
		if errors.Is(err, redis.Nil) {
			return nil
		}
		if err != nil {
			if isRedisUnavailable(err) {
				return fmt.Errorf("认领秒杀Pending：%w", ErrRedisUnavailable)
			}
			return fmt.Errorf("认领秒杀Pending：%w", err)
		}
		for _, msg := range messages {
			s.processSeckillMessage(ctx, msg)
		}
		if next == "0-0" || len(messages) == 0 {
			return nil
		}
		start = next
	}
}

func (s *VoucherService) readSeckillMessages(
	ctx context.Context,
	consumer string,
	id string,
	block time.Duration,
) error {
	streams, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    seckillConsumerGroup,
		Consumer: consumer,
		Streams:  []string{redisx.OrderStream, id},
		Count:    10,
		Block:    block,
	}).Result()

	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		if isRedisUnavailable(err) {
			return fmt.Errorf("读取秒杀消息：%w", ErrRedisUnavailable)
		}
		return fmt.Errorf("读取秒杀消息：%w", err)
	}
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			s.processSeckillMessage(ctx, msg)
		}
	}
	return nil
}

func (s *VoucherService) processSeckillMessage(
	ctx context.Context,
	msg redis.XMessage,
) {
	err := s.handleSeckillMessage(ctx, msg)
	switch {
	case err == nil:
		s.ackSeckillMessage(ctx, msg.ID)
	case errors.Is(err, errPoisonSeckillMsg):
		if _, parseErr := messageInt64(msg.Values, "orderId"); parseErr != nil {
			log.Printf("丢弃无法解析的秒杀消息，id=%s：%v", msg.ID, err)
			s.ackSeckillMessage(ctx, msg.ID)
			return
		}
		if err := s.compensateSeckill(ctx, msg); err != nil {
			log.Printf("秒杀补偿失败，id=%s：%v", msg.ID, err)
			return
		}
		if err := s.deadLetterSeckill(ctx, msg, err); err != nil {
			log.Printf("秒杀消息写入死信失败，id=%s：%v", msg.ID, err)
			return
		}
		s.ackSeckillMessage(ctx, msg.ID)
	case errors.Is(err, repository.ErrVoucherOutOfStock):
		if err := s.compensateSeckill(ctx, msg); err != nil {
			log.Printf("秒杀补偿失败，id=%s：%v", msg.ID, err)
			return
		}
		s.ackSeckillMessage(ctx, msg.ID)
	case errors.Is(err, ErrMySQLUnavailable),
		errors.Is(err, ErrRedisUnavailable):
		log.Printf("秒杀消息暂不确认，id=%s：%v", msg.ID, err)
	default:
		if s.seckillDeliveryCount(ctx, msg.ID) >= seckillMaxDeliveries {
			log.Printf(
				"秒杀消息多次失败，转为确定失败，id=%s：%v",
				msg.ID,
				err,
			)
			if err := s.compensateSeckill(ctx, msg); err != nil {
				log.Printf("秒杀补偿失败，id=%s：%v", msg.ID, err)
				return
			}
			if err := s.deadLetterSeckill(ctx, msg, err); err != nil {
				log.Printf("秒杀消息写入死信失败，id=%s：%v", msg.ID, err)
				return
			}
			s.ackSeckillMessage(ctx, msg.ID)
			return
		}
		log.Printf("秒杀消息处理失败，保留Pending，id=%s：%v", msg.ID, err)
	}
}

// deadLetterSeckill records a terminally failed message before the original
// Pending entry is ACKed. The payload remains available for inspection or a
// later replay without allowing the worker to retry it indefinitely.
func (s *VoucherService) deadLetterSeckill(
	ctx context.Context,
	msg redis.XMessage,
	reason error,
) error {
	values := make(map[string]interface{}, len(msg.Values)+3)
	for key, value := range msg.Values {
		values[key] = value
	}
	values["originalId"] = msg.ID
	values["reason"] = reason.Error()
	values["deliveries"] = s.seckillDeliveryCount(ctx, msg.ID)
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: redisx.OrderDeadLetterStream,
		Values: values,
		MaxLen: 10000,
		Approx: true,
	}).Err()
}

func (s *VoucherService) seckillDeliveryCount(
	ctx context.Context,
	id string,
) int64 {
	pending, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: redisx.OrderStream,
		Group:  seckillConsumerGroup,
		Start:  id,
		End:    id,
		Count:  1,
	}).Result()
	if err != nil || len(pending) == 0 {
		return 0
	}
	return pending[0].RetryCount
}

func (s *VoucherService) ackSeckillMessage(
	ctx context.Context,
	id string,
) {
	if err := s.rdb.XAck(
		ctx,
		redisx.OrderStream,
		seckillConsumerGroup,
		id,
	).Err(); err != nil {
		// 落单或补偿已经完成。ACK 失败时消息会回到 Pending，
		// 下次按幂等重放，不能在这里退库存。
		log.Printf("秒杀消息 ACK 失败，保留Pending，id=%s：%v", id, err)
	}
}

func messageInt64(
	values map[string]interface{},
	key string,
) (int64, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("消息缺少字段：%s", key)
	}

	n, err := strconv.ParseInt(fmt.Sprint(raw), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("消息字段无效：%s", key)
	}
	return n, nil
}

func (s *VoucherService) handleSeckillMessage(
	ctx context.Context,
	msg redis.XMessage,
) error {
	orderID, err := messageInt64(msg.Values, "orderId")
	if err != nil {
		return fmt.Errorf("%w：%v", errPoisonSeckillMsg, err)
	}
	userID, err := messageInt64(msg.Values, "userId")
	if err != nil {
		return fmt.Errorf("%w：%v", errPoisonSeckillMsg, err)
	}
	voucherID, err := messageInt64(msg.Values, "voucherId")
	if err != nil {
		return fmt.Errorf("%w：%v", errPoisonSeckillMsg, err)
	}

	existing, err := s.repo.FindVoucherOrderByID(ctx, orderID)
	if err == nil {
		if existing.UserID != userID || existing.VoucherID != voucherID {
			return fmt.Errorf(
				"%w：订单ID已存在但不匹配当前消息",
				errPoisonSeckillMsg,
			)
		}
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		if isMySQLUnavailable(err) {
			return fmt.Errorf("查询秒杀订单：%w", ErrMySQLUnavailable)
		}
		return fmt.Errorf("查询秒杀订单：%w", err)
	}

	order := &model.VoucherOrder{
		ID:        orderID,
		UserID:    userID,
		VoucherID: voucherID,
		PayType:   1,
		Status:    1,
	}

	err = s.repo.CreateVoucherOrder(ctx, order)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrVoucherPurchased):
		return nil
	case errors.Is(err, repository.ErrVoucherOutOfStock):
		return err
	case isMySQLUnavailable(err):
		// 提交结果不确定时不能补偿库存，只能保留消息重试。
		return fmt.Errorf("创建秒杀订单：%w", ErrMySQLUnavailable)
	default:
		return fmt.Errorf("创建秒杀订单：%w", err)
	}
}

func (s *VoucherService) compensateSeckill(
	ctx context.Context,
	msg redis.XMessage,
) error {
	orderID, err := messageInt64(msg.Values, "orderId")
	if err != nil {
		return err
	}
	userID, err := messageInt64(msg.Values, "userId")
	if err != nil {
		return err
	}
	voucherID, err := messageInt64(msg.Values, "voucherId")
	if err != nil {
		return err
	}

	result, err := redisx.CompensateScript.Run(
		ctx,
		s.rdb,
		[]string{
			redisx.Key(redisx.SeckillStockPrefix, voucherID),
			redisx.Key(redisx.SeckillBuyersPrefix, voucherID),
			redisx.Key(redisx.OrderStatusPrefix, orderID),
			redisx.Key(redisx.SeckillCompensatedPrefix, orderID),
		},
		userID,
		orderID,
	).Int()
	if err != nil {
		if isRedisUnavailable(err) {
			return fmt.Errorf("执行秒杀补偿脚本：%w", ErrRedisUnavailable)
		}
		return fmt.Errorf("执行秒杀补偿脚本：%w", err)
	}
	if result < 0 {
		return fmt.Errorf("秒杀补偿预检失败：%d", result)
	}
	return nil
}
