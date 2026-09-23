package dto

import "time"

type VoucherCreateRequest struct {
	ShopID      int64  `json:"shopId"`
	Title       string `json:"title"`
	SubTitle    string `json:"subTitle"`
	Rules       string `json:"rules"`
	PayValue    int64  `json:"payValue"`
	ActualValue int64  `json:"actualValue"`
}

type SeckillVoucherCreateRequest struct {
	VoucherCreateRequest

	Stock     int       `json:"stock"`
	BeginTime time.Time `json:"beginTime"`
	EndTime   time.Time `json:"endTime"`
}
