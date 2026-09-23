package redisx

import "strconv"

const (
	SeckillStockPrefix  = "seckill:{seckill}:stock:"
	SeckillMetaPrefix   = "seckill:{seckill}:meta:"
	SeckillBuyersPrefix = "seckill:{seckill}:buyers:"
	OrderStatusPrefix   = "seckill:{seckill}:status:"

	OrderStream              = "stream:{seckill}:orders"
	OrderDeadLetterStream    = "stream:{seckill}:orders:dead-letter"
	SeckillCompensatedPrefix = "seckill:{seckill}:compensated:"
)

func Key(prefix string, id int64) string {
	return prefix + strconv.FormatInt(id, 10)
}
func SeckillKeys(voucherID int64) []string {
	return []string{
		Key(SeckillStockPrefix, voucherID),
		Key(SeckillMetaPrefix, voucherID),
		Key(SeckillBuyersPrefix, voucherID),
	}
}

func SeckillOrderKeys(voucherID, orderID int64) []string {
	return []string{
		Key(SeckillStockPrefix, voucherID),
		Key(SeckillMetaPrefix, voucherID),
		Key(SeckillBuyersPrefix, voucherID),
		OrderStream,
		Key(OrderStatusPrefix, orderID),
	}
}
