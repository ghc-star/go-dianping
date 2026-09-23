package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/learning/go-dianping/internal/dto"
	"github.com/learning/go-dianping/internal/middleware"
	"github.com/learning/go-dianping/internal/model"
	"github.com/learning/go-dianping/internal/service"
)

type VoucherHandler struct {
	voucher *service.VoucherService
}

func NewVoucherHandler(
	voucher *service.VoucherService,
) *VoucherHandler {
	return &VoucherHandler{voucher: voucher}
}

func (h *VoucherHandler) Create(c *gin.Context) {
	var in dto.VoucherCreateRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "优惠券请求参数错误",
		})
		return
	}
	voucher := model.Voucher{
		ShopID:      in.ShopID,
		Title:       in.Title,
		SubTitle:    in.SubTitle,
		Rules:       in.Rules,
		PayValue:    in.PayValue,
		ActualValue: in.ActualValue,
	}
	id, err := h.voucher.Create(
		c.Request.Context(),
		&voucher,
	)
	if errors.Is(err, service.ErrInvalidVoucher) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	}
	if err != nil {
		log.Printf("创建优惠券失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "创建优惠券失败",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    id,
	})
}

func (h *VoucherHandler) ListByShop(c *gin.Context) {
	shopID, err := strconv.ParseInt(
		c.Param("shopId"),
		10,
		64,
	)

	if err != nil || shopID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "shopId必须是正整数",
		})
		return
	}

	vouchers, err := h.voucher.ListByShop(
		c.Request.Context(),
		shopID,
	)
	if errors.Is(err, service.ErrInvalidVoucher) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	}
	if err != nil {
		log.Printf("查询优惠券失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询优惠券失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    vouchers,
	})
}

func (h *VoucherHandler) CreateSeckill(c *gin.Context) {
	var in dto.SeckillVoucherCreateRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "秒杀券请求参数错误",
		})
		return
	}

	voucher := model.Voucher{
		ShopID:      in.ShopID,
		Title:       in.Title,
		SubTitle:    in.SubTitle,
		Rules:       in.Rules,
		PayValue:    in.PayValue,
		ActualValue: in.ActualValue,
	}

	campaign := model.SeckillVoucher{
		Stock:     in.Stock,
		BeginTime: in.BeginTime,
		EndTime:   in.EndTime,
	}

	id, err := h.voucher.CreateSeckill(
		c.Request.Context(),
		&voucher,
		&campaign,
	)

	if errors.Is(err, service.ErrInvalidVoucher) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	}
	if err != nil {
		log.Printf("创建秒杀券失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "创建秒杀券失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    id,
	})
}

func (h *VoucherHandler) Seckill(c *gin.Context) {
	voucherID, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)
	if err != nil || voucherID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "优惠券ID必须是正整数",
		})
		return
	}
	user := middleware.CurrentUser(c)
	if user == nil || user.ID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}
	orderID, err := h.voucher.SeckillAsync(
		c.Request.Context(),
		voucherID,
		user.ID,
	)
	switch {
	case errors.Is(err, service.ErrInvalidVoucher):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	case errors.Is(err, service.ErrSeckillNotStarted),
		errors.Is(err, service.ErrSeckillEnded):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	case errors.Is(err, service.ErrSeckillOutOfStock),
		errors.Is(err, service.ErrAlreadyPurchased):
		c.JSON(http.StatusConflict, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	case errors.Is(err, service.ErrRedisUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success":  false,
			"errorMsg": "秒杀服务暂时不可用",
		})
		return

	case err != nil:
		log.Printf(
			"秒杀下单失败，voucherID=%d userID=%d：%v",
			voucherID,
			user.ID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "秒杀下单失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    orderID,
	})
}

func (h *VoucherHandler) FindOrder(c *gin.Context) {
	orderID, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)
	if err != nil || orderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "订单ID必须是正整数",
		})
		return
	}

	user := middleware.CurrentUser(c)
	if user == nil || user.ID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}

	order, err := h.voucher.FindOrder(
		c.Request.Context(),
		orderID,
		user.ID,
	)

	switch {
	case errors.Is(err, service.ErrInvalidVoucher):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return

	case errors.Is(err, service.ErrVoucherOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "订单不存在",
		})
		return
	case errors.Is(err, service.ErrMySQLUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success":  false,
			"errorMsg": "订单暂时不可查询",
		})
		return

	case err != nil:
		log.Printf(
			"查询订单失败，orderID=%d userID=%d：%v",
			orderID,
			user.ID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询订单失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    order,
	})
}

func (h *VoucherHandler) FindOrderStatus(c *gin.Context) {
	orderID, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)
	if err != nil || orderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "订单ID必须是正整数",
		})
		return
	}

	user := middleware.CurrentUser(c)
	if user == nil || user.ID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}

	status, err := h.voucher.FindOrderStatus(
		c.Request.Context(),
		orderID,
		user.ID,
	)
	switch {
	case errors.Is(err, service.ErrInvalidVoucher):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	case errors.Is(err, service.ErrVoucherOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "订单不存在",
		})
		return
	case errors.Is(err, service.ErrRedisUnavailable),
		errors.Is(err, service.ErrMySQLUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success":  false,
			"errorMsg": "订单状态暂时不可查询",
		})
		return
	case err != nil:
		log.Printf(
			"查询订单状态失败，orderID=%d userID=%d：%v",
			orderID,
			user.ID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询订单状态失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}
