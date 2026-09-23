package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/learning/go-dianping/internal/middleware"
)

type UploadHandler struct {
	directory string
	maxBytes  int64
}

func NewUploadHandler(directory string, maxBytes int64) *UploadHandler {
	return &UploadHandler{
		directory: directory,
		maxBytes:  maxBytes,
	}
}

func (h *UploadHandler) UploadBlog(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		h.maxBytes+64*1024,
	)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "file必填或图片过大",
		})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "读取上传图片失败",
		})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, h.maxBytes+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "读取上传图片失败",
		})
		return
	}
	if int64(len(data)) > h.maxBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "图片超过大小限制",
		})
		return
	}

	contentType := http.DetectContentType(data)
	var extension string
	switch contentType {
	case "image/png":
		extension = "png"
	case "image/jpeg":
		extension = "jpg"
	case "image/gif":
		extension = "gif"
	case "image/webp":
		extension = "webp"
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "只支持PNG、JPEG、GIF、WebP图片",
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

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "生成文件名失败",
		})
		return
	}

	randomName := hex.EncodeToString(randomBytes)
	name := fmt.Sprintf(
		"/blogs/%d/%s.%s",
		user.ID,
		randomName,
		extension,
	)

	userDirectory := filepath.Join(
		h.directory,
		"blogs",
		strconv.FormatInt(user.ID, 10),
	)

	if err := os.MkdirAll(userDirectory, 0750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "创建上传目录失败",
		})
		return
	}

	filePath := filepath.Join(
		userDirectory,
		randomName+"."+extension,
	)

	destination, err := os.OpenFile(
		filePath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0640,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "创建图片文件失败",
		})
		return
	}

	if _, err := destination.Write(data); err != nil {
		destination.Close()
		os.Remove(filePath)

		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "保存图片失败",
		})
		return
	}

	if err := destination.Close(); err != nil {
		os.Remove(filePath)

		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "保存图片失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    name,
	})
}

var uploadedImagePattern = regexp.MustCompile(
	`^/blogs/([1-9][0-9]*)/[a-f0-9]{64}\.(png|jpg|gif|webp)$`,
)

func (h *UploadHandler) DeleteBlog(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success":  false,
			"errorMsg": "请先登录",
		})
		return
	}

	name := c.Query("name")
	matches := uploadedImagePattern.FindStringSubmatch(name)
	if len(matches) == 0 ||
		matches[1] != strconv.FormatInt(user.ID, 10) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":  false,
			"errorMsg": "文件名无效或图片不属于当前用户",
		})
		return
	}
	relativePath := strings.TrimPrefix(name, "/")
	filePath := filepath.Join(
		h.directory,
		filepath.FromSlash(relativePath),
	)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success":  false,
				"errorMsg": "图片不存在",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"errorMsg": "删除图片失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}
