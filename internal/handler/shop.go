package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/learning/go-dianping/internal/dto"
	"github.com/learning/go-dianping/internal/model"
	"github.com/learning/go-dianping/internal/service"
	"gorm.io/gorm"
)

type ShopHandler struct {
	shop *service.ShopService
}

func NewShopHandler(shop *service.ShopService) *ShopHandler {
	return &ShopHandler{shop: shop}
}

func (h *ShopHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "id必须是正整数",
		})
		return
	}
	result, err := h.shop.GetByID(c.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "商铺不存在",
		})
		return
	}
	if err != nil {
		log.Printf("查询商铺失败，id=%d：%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询商铺失败",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func (h *ShopHandler) ShopCreate(c *gin.Context) {
	var in dto.ShopInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "商铺请求参数错误",
		})
		return
	}

	// 创建商铺时，这些字段必须提供。
	if in.Name == nil ||
		in.TypeID == nil ||
		in.Address == nil ||
		in.X == nil ||
		in.Y == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "name、typeId、address、x、y必填",
		})
		return
	}
	var shop model.Shop
	in.ApplyTo(&shop)
	id, err := h.shop.ShopCreate(c.Request.Context(), &shop)
	if errors.Is(err, service.ErrInvalidShopCreat) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	}
	if err != nil {
		log.Printf("创建商铺失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "创建商铺失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    id,
	})
}

func (h *ShopHandler) ShopUpdate(c *gin.Context) {
	var in dto.ShopInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "商铺请求参数错误",
		})
		return
	}
	err := h.shop.ShopUpdate(c.Request.Context(), in)
	if err != nil {
		log.Printf("更新商铺失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "更新商铺失败",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

func (h *ShopHandler) ListByType(c *gin.Context) {
	typeID, err := strconv.ParseInt(
		c.Query("typeId"),
		10,
		64,
	)
	if err != nil || typeID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "typeId必须是正整数",
		})
		return
	}
	current := 1
	currentText := c.Query("current")
	if currentText != "" {
		current, err = strconv.Atoi(currentText)
		if err != nil || current <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "current必须是正整数",
			})
			return
		}
	}
	xText := c.Query("x")
	yText := c.Query("y")
	var x *float64
	var y *float64
	if xText == "" && yText == "" {

	} else {
		if xText == "" || yText == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "x和y必须同时提供",
			})
			return
		}
		xx, err1 := strconv.ParseFloat(xText, 64)
		yy, err2 := strconv.ParseFloat(yText, 64)

		if err1 != nil || err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "x和y必须是数字",
			})
			return
		}
		x = &xx
		y = &yy
	}
	shops, err := h.shop.ListByType(
		c.Request.Context(),
		typeID,
		current,
		x,
		y,
	)
	if err != nil {
		log.Printf("查询分类商铺失败：%v", err)

		if errors.Is(err, service.ErrInvalidShopQuery) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": err.Error(),
			})
			return
		}

		if errors.Is(err, service.ErrGeoUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success":  false,
				"errorMsg": "附近商铺服务暂时不可用",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询商铺失败",
		})
		return
	}

	// 5. 返回结果
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    shops,
	})
}
