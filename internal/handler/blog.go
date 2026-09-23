package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/learning/go-dianping/internal/dto"
	"github.com/learning/go-dianping/internal/middleware"
	"github.com/learning/go-dianping/internal/service"
	"gorm.io/gorm"
)

type BlogHandler struct {
	blogs *service.BlogService
}

func NewBlogHandler(blogs *service.BlogService) *BlogHandler {
	return &BlogHandler{blogs: blogs}
}

func (h *BlogHandler) Create(c *gin.Context) {
	var input dto.BlogCreateRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "笔记请求参数错误",
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

	id, err := h.blogs.Create(
		c.Request.Context(),
		user.ID,
		input,
	)
	switch {
	case errors.Is(err, service.ErrInvalidBlog):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "商铺不存在",
		})
		return

	case err != nil:
		log.Printf("发布笔记失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "发布笔记失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    id,
	})
}

func (h *BlogHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "笔记ID必须是正整数",
		})
		return
	}
	blog, err := h.blogs.GetByID(
		c.Request.Context(),
		id,
	)
	switch {
	case errors.Is(err, service.ErrInvalidBlog):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "笔记或作者不存在",
		})
		return

	case err != nil:
		log.Printf("查询笔记失败，id=%d：%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询笔记失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    blog,
	})

}

func (h *BlogHandler) ListMine(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}
	current := 1
	if value := c.Query("current"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "current必须是正整数",
			})
			return
		}
		current = page
	}
	blogs, err := h.blogs.ListByUser(
		c.Request.Context(),
		user.ID,
		current,
	)
	if errors.Is(err, service.ErrInvalidBlog) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	}
	if err != nil {
		log.Printf(
			"查询我的笔记失败，userID=%d：%v",
			user.ID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询我的笔记失败",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    blogs,
	})
}

func (h *BlogHandler) ListByUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "id必须是正整数",
		})
		return
	}
	current := 1
	if value := c.Query("current"); value != "" {
		current, err = strconv.Atoi(value)
		if err != nil || current <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "current必须是正整数",
			})
			return
		}
	}
	blogs, err := h.blogs.ListByUser(
		c.Request.Context(),
		userID,
		current,
	)
	if errors.Is(err, service.ErrInvalidBlog) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	}
	if err != nil {
		log.Printf(
			"查询用户笔记失败，userID=%d：%v",
			userID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询用户笔记失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    blogs,
	})
}

func (h *BlogHandler) ListHot(c *gin.Context) {
	current := 1
	if value := c.Query("current"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "current必须是正整数",
			})
			return
		}
		current = page
	}
	blogs, err := h.blogs.ListHot(
		c.Request.Context(),
		current,
	)
	switch {
	case errors.Is(err, service.ErrInvalidBlog):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "笔记作者不存在",
		})
		return

	case err != nil:
		log.Printf("查询热门笔记失败：%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询热门笔记失败",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    blogs,
	})
}

func (h *BlogHandler) ToggleLike(c *gin.Context) {
	blogID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || blogID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "笔记ID必须是正整数",
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
	err = h.blogs.ToggleLike(
		c.Request.Context(),
		user.ID,
		blogID,
	)
	switch {
	case errors.Is(err, service.ErrInvalidBlog):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return
	case errors.Is(err, service.ErrBlogLikeBusy):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success":  false,
			"errorMsg": "点赞操作繁忙，请稍后再试",
		})
		return
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "笔记不存在",
		})
		return
	case err != nil:
		log.Printf(
			"切换笔记点赞失败，blogID=%d userID=%d：%v",
			blogID,
			user.ID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "点赞失败",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

func (h *BlogHandler) ListEarliestLikers(c *gin.Context) {
	blogID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || blogID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "笔记ID必须是正整数",
		})
		return
	}

	users, err := h.blogs.ListEarliestLikers(
		c.Request.Context(),
		blogID,
	)
	switch {
	case errors.Is(err, service.ErrInvalidBlog):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})
		return

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success":  false,
			"errorMsg": "笔记或点赞用户不存在",
		})
		return

	case err != nil:
		log.Printf("查询笔记点赞用户失败，blogID=%d：%v", blogID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "查询点赞用户失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    users,
	})
}
func (h *BlogHandler) Feed(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}

	// 第一次请求默认从当前时间开始向过去读取。
	lastID := time.Now().UnixMilli()
	if value := c.Query("lastId"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "lastId必须是非负整数",
			})
			return
		}
		lastID = parsed
	}

	offset := 0
	if value := c.Query("offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success":  false,
				"errorMsg": "offset必须是非负整数",
			})
			return
		}
		offset = parsed
	}

	result, err := h.blogs.Feed(
		c.Request.Context(),
		user.ID,
		lastID,
		offset,
	)

	switch {
	case errors.Is(err, service.ErrInvalidBlog):
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": err.Error(),
		})

	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "Feed中的笔记或作者不存在",
		})

	case err != nil:
		log.Printf("查询关注Feed失败，userID=%d：%v", user.ID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success":  false,
			"errorMsg": "Feed暂时不可用",
		})

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    result,
		})
	}
}
