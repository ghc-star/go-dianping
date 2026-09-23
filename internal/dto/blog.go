package dto

import "github.com/learning/go-dianping/internal/model"

type BlogCreateRequest struct {
	ShopID  int64  `json:"shopId"`
	Title   string `json:"title"`
	Images  string `json:"images"`
	Content string `json:"content"`
}

type ScrollResult struct {
	List    []model.Blog `json:"list"`
	MinTime int64        `json:"minTime"`
	Offset  int          `json:"offset"`
}
