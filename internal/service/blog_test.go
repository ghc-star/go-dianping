package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/learning/go-dianping/internal/dto"
)

func TestValidateBlogCreate(t *testing.T) {
	validInput := dto.BlogCreateRequest{
		ShopID:  1,
		Title:   "探店笔记",
		Images:  "/imgs/blogs/1.png",
		Content: "这家店很好吃",
	}
	tests := []struct {
		name   string
		userID int64
		change func(*dto.BlogCreateRequest)
		valid  bool
	}{
		{
			name:   "合法参数",
			userID: 1,
			valid:  true,
		},
		{
			name:   "用户ID无效",
			userID: 0,
		},
		{
			name:   "商铺ID无效",
			userID: 1,
			change: func(in *dto.BlogCreateRequest) {
				in.ShopID = 0
			},
		},
		{
			name:   "标题为空",
			userID: 1,
			change: func(in *dto.BlogCreateRequest) {
				in.Title = "   "
			},
		},
		{
			name:   "标题超过255字",
			userID: 1,
			change: func(in *dto.BlogCreateRequest) {
				in.Title = strings.Repeat("店", 256)
			},
		},
		{
			name:   "正文为空",
			userID: 1,
			change: func(in *dto.BlogCreateRequest) {
				in.Content = ""
			},
		},
		{
			name:   "没有图片",
			userID: 1,
			change: func(in *dto.BlogCreateRequest) {
				in.Images = ""
			},
		},
		{
			name:   "图片超过9张",
			userID: 1,
			change: func(in *dto.BlogCreateRequest) {
				in.Images = strings.Join([]string{
					"1", "2", "3", "4", "5",
					"6", "7", "8", "9", "10",
				}, ",")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validInput
			if tt.change != nil {
				tt.change(&input)
			}

			err := validateBlogCreate(tt.userID, input)

			if tt.valid {
				if err != nil {
					t.Fatalf("合法参数不应报错：%v", err)
				}
				return
			}

			if !errors.Is(err, ErrInvalidBlog) {
				t.Fatalf(
					"期望 ErrInvalidBlog，实际得到：%v",
					err,
				)
			}
		})
	}
}
