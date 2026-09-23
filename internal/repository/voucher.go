package repository

import (
	"context"
	"errors"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/learning/go-dianping/internal/model"
	"gorm.io/gorm"
)

func (r *Repository) VoucherCreate(
	ctx context.Context,
	voucher *model.Voucher,
) error {
	return r.DB.WithContext(ctx).Create(voucher).Error
}

func (r *Repository) VoucherListByShop(
	ctx context.Context,
	shopID int64,
) ([]model.Voucher, error) {
	type voucherView struct {
		model.Voucher
		Stock     *int       `gorm:"column:stock"`
		BeginTime *time.Time `gorm:"column:begin_time"`
		EndTime   *time.Time `gorm:"column:end_time"`
	}

	rows := make([]voucherView, 0)

	err := r.DB.
		WithContext(ctx).
		Table("tb_voucher AS v").
		Select(`
			v.*,
			sv.stock AS stock,
			sv.begin_time AS begin_time,
			sv.end_time AS end_time
	`).
		Joins(`
			LEFT JOIN tb_seckill_voucher AS sv
			ON sv.voucher_id = v.id
		`).
		Where("v.shop_id = ? AND v.status = ?", shopID, 1).
		Order("v.id ASC").
		Scan(&rows).
		Error
	if err != nil {
		return nil, err
	}

	vouchers := make([]model.Voucher, 0, len(rows))
	for _, row := range rows {
		row.Voucher.Stock = row.Stock
		row.Voucher.BeginTime = row.BeginTime
		row.Voucher.EndTime = row.EndTime

		vouchers = append(vouchers, row.Voucher)
	}

	return vouchers, nil
}

func (r *Repository) SeckillVoucherCreate(
	ctx context.Context,
	voucher *model.Voucher,
	campaign *model.SeckillVoucher,
) error {
	return r.DB.WithContext(ctx).Transaction(
		func(tx *gorm.DB) error {
			if err := tx.Create(voucher).Error; err != nil {
				return err
			}

			//第一条INSERT成功后，GORM已把自增ID
			//回填到voucher.ID
			campaign.VoucherID = voucher.ID

			if err := tx.Create(campaign).Error; err != nil {
				return err
			}
			return nil
		},
	)
}

var (
	ErrVoucherOutOfStock = errors.New("秒杀券库存不足")
	ErrVoucherPurchased  = errors.New("用户已经购买该优惠券")
)

func (r *Repository) CreateVoucherOrder(
	ctx context.Context,
	order *model.VoucherOrder,
) error {
	return r.DB.WithContext(ctx).Transaction(
		func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&model.VoucherOrder{}).
				Where(
					"user_id = ? AND voucher_id = ?",
					order.UserID,
					order.VoucherID,
				).
				Count(&count).
				Error; err != nil {
				return err
			}

			if count > 0 {
				return ErrVoucherPurchased
			}

			result := tx.Model(&model.SeckillVoucher{}).
				Where(
					"voucher_id = ? AND stock > 0",
					order.VoucherID,
				).
				UpdateColumn("stock", gorm.Expr("stock - 1"))

			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrVoucherOutOfStock
			}

			if err := tx.Create(order).Error; err != nil {
				var mysqlErr *mysqlDriver.MySQLError
				if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
					return ErrVoucherPurchased
				}
				if errors.Is(err, gorm.ErrDuplicatedKey) {
					return ErrVoucherPurchased
				}
				return err
			}

			return nil

		},
	)
}

func (r *Repository) FindSeckillVoucher(
	ctx context.Context,
	voucherID int64,
) (model.SeckillVoucher, error) {
	var campaign model.SeckillVoucher

	err := r.DB.
		WithContext(ctx).
		Where("voucher_id = ?", voucherID).
		First(&campaign).
		Error

	return campaign, err
}

func (r *Repository) FindVoucherOrder(
	ctx context.Context,
	orderID int64,
	userID int64,
) (model.VoucherOrder, error) {
	var order model.VoucherOrder

	err := r.DB.
		WithContext(ctx).
		Where(
			"id = ? AND user_id = ?",
			orderID,
			userID,
		).
		First(&order).
		Error

	return order, err
}

func (r *Repository) FindVoucherOrderByID(
	ctx context.Context,
	orderID int64,
) (model.VoucherOrder, error) {
	var order model.VoucherOrder
	err := r.DB.
		WithContext(ctx).
		Where("id = ?", orderID).
		First(&order).
		Error
	return order, err
}
