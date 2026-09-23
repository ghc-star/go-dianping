package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/learning/go-dianping/internal/dto"
	"github.com/learning/go-dianping/internal/middleware"
	"github.com/learning/go-dianping/internal/service"
	"gorm.io/gorm"
)

type FollowHandler struct {
	follows *service.FollowService
}

func NewFollowHandler(follows *service.FollowService) *FollowHandler {
	return &FollowHandler{follows: follows}
}

func (h *FollowHandler) Follow(c *gin.Context) {
	var input dto.FollowRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "关注请求参数错误",
		})
		return
	}
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}
	err := h.follows.Follow(
		c.Request.Context(),
		user.ID,
		input.FollowUserID,
		*input.IsFollow,
	)
	switch {
	case errors.Is(err, service.ErrInvalidFollow):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "目标用户不存在",
		})

	case err != nil:
		log.Printf("修改关注关系失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "修改关注关系失败",
		})

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": true,
		})
	}
}

func (h *FollowHandler) IsFollow(c *gin.Context) {
	var input dto.FollowTargetRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "查询关注请求参数错误",
		})
		return
	}
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}
	isFollow, err := h.follows.IsFollow(
		c.Request.Context(),
		user.ID,
		input.FollowUserID,
	)

	switch {
	case errors.Is(err, service.ErrInvalidFollow):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "目标用户不存在",
		})

	case err != nil:
		log.Printf("查询关注关系失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询关注关系失败",
		})

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    isFollow,
		})
	}
}

func (h *FollowHandler) CommonFollows(c *gin.Context) {
	var input dto.FollowTargetRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "共同关注请求参数错误",
		})
		return
	}
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}
	users, err := h.follows.CommonFollows(
		c.Request.Context(),
		user.ID,
		input.FollowUserID,
	)
	switch {
	case errors.Is(err, service.ErrInvalidFollow):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "目标用户不存在",
		})

	case err != nil:
		log.Printf("查询共同关注失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询共同关注失败",
		})

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    users,
		})
	}
}
