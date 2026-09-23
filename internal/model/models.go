package model

import "time"

type Shop struct {
	Id         int64     `json:"id" gorm:"column:id;primaryKey"`
	Name       string    `json:"name" gorm:"column:name"`
	TypeID     int64     `gorm:"column:type_id" json:"typeId"`
	Images     string    `gorm:"column:images;size:1024" json:"images"`
	Area       string    `gorm:"column:area" json:"area"`
	Address    string    `gorm:"column:address;size:255" json:"address"`
	X          float64   `gorm:"column:x" json:"x"`
	Y          float64   `gorm:"column:y" json:"y"`
	AvgPrice   int64     `gorm:"column:avg_price" json:"avgPrice"`
	Sold       int       `gorm:"column:sold" json:"sold"`
	Comments   int       `gorm:"column:comments" json:"comments"`
	Score      int       `gorm:"column:score" json:"score"`
	OpenHours  string    `gorm:"column:open_hours" json:"openHours"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Distance   *float64  `gorm:"-" json:"distance,omitempty"`
}

func (Shop) TableName() string {
	return "tb_shop"
}

type User struct {
	ID         int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Phone      string    `json:"phone,omitempty" gorm:"column:phone;size:11"`
	Password   string    `json:"-" gorm:"column:password;size:128"`
	NickName   string    `gorm:"column:nick_name;size:32" json:"nickName"`
	Icon       string    `gorm:"column:icon;size:255" json:"icon"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
}

func (User) TableName() string {
	return "tb_user"
}

type UserInfo struct {
	UserID     int64      `gorm:"column:user_id;primaryKey;autoIncrement:false" json:"userId"`
	City       string     `gorm:"column:city" json:"city"`
	Introduce  string     `gorm:"column:introduce" json:"introduce"`
	Fans       int        `gorm:"column:fans" json:"fans"`
	Followee   int        `gorm:"column:followee" json:"followee"`
	Gender     int        `gorm:"column:gender" json:"gender"`
	Birthday   *time.Time `gorm:"column:birthday;type:date" json:"birthday,omitempty"`
	Credits    int        `gorm:"column:credits" json:"credits"`
	Level      int        `gorm:"column:level" json:"level"`
	CreateTime time.Time  `gorm:"column:create_time;autoCreateTime" json:"-"`
	UpdateTime time.Time  `gorm:"column:update_time;autoUpdateTime" json:"-"`
}

func (UserInfo) TableName() string {
	return "tb_user_info"
}

type ShopType struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name       string    `gorm:"column:name" json:"name"`
	Icon       string    `gorm:"column:icon" json:"icon"`
	Sort       int       `gorm:"column:sort" json:"sort"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"-"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"-"`
}

func (ShopType) TableName() string {
	return "tb_shop_type"
}

type Blog struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ShopID     int64     `gorm:"column:shop_id" json:"shopId"`
	UserID     int64     `gorm:"column:user_id" json:"userId"`
	Title      string    `gorm:"column:title;size:255" json:"title"`
	Images     string    `gorm:"column:images;size:2048" json:"images"`
	Content    string    `gorm:"column:content;size:2048" json:"content"`
	Liked      int       `gorm:"column:liked" json:"liked"`
	Comments   int       `gorm:"column:comments" json:"comments"`
	Name       string    `gorm:"-" json:"name"`
	Icon       string    `gorm:"-" json:"icon"`
	IsLike     bool      `gorm:"-" json:"isLike"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
}

func (Blog) TableName() string {
	return "tb_blog"
}

type BlogLike struct {
	BlogID     int64     `gorm:"column:blog_id;primaryKey;autoIncrement:false" json:"blogId"`
	UserID     int64     `gorm:"column:user_id;primaryKey;autoIncrement:false" json:"userId"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
}

func (BlogLike) TableName() string {
	return "tb_blog_like"
}

type Follow struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID       int64     `gorm:"column:user_id;uniqueIndex:uk_user_follow" json:"userId"`
	FollowUserID int64     `gorm:"column:follow_user_id;uniqueIndex:uk_user_follow" json:"followUserId"`
	CreateTime   time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
}

func (Follow) TableName() string {
	return "tb_follow"
}

type FeedOutbox struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID      int64      `gorm:"column:user_id;uniqueIndex:uk_feed_user_blog" json:"userId"`
	BlogID      int64      `gorm:"column:blog_id;uniqueIndex:uk_feed_user_blog" json:"blogId"`
	Score       int64      `gorm:"column:score" json:"score"`
	DeliveredAt *time.Time `gorm:"column:delivered_at" json:"deliveredAt,omitempty"`
	CreateTime  time.Time  `gorm:"column:create_time;autoCreateTime" json:"createTime"`
}

func (FeedOutbox) TableName() string {
	return "tb_feed_outbox"
}

type Voucher struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ShopID      int64     `gorm:"column:shop_id" json:"shopId"`
	Title       string    `gorm:"column:title;size:255" json:"title"`
	SubTitle    string    `gorm:"column:sub_title;size:255" json:"subTitle"`
	Rules       string    `gorm:"column:rules;size:1024" json:"rules"`
	PayValue    int64     `gorm:"column:pay_value" json:"payValue"`
	ActualValue int64     `gorm:"column:actual_value" json:"actualValue"`
	Type        int       `gorm:"column:type" json:"type"`
	Status      int       `gorm:"column:status" json:"status"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`

	Stock     *int       `gorm:"-" json:"stock,omitempty"`
	BeginTime *time.Time `gorm:"-" json:"beginTime,omitempty"`
	EndTime   *time.Time `gorm:"-" json:"endTime,omitempty"`
}

func (Voucher) TableName() string {
	return "tb_voucher"
}

type SeckillVoucher struct {
	VoucherID int64 `gorm:"column:voucher_id;primaryKey;autoIncrement:false" json:"voucherId"`
	Stock     int   `gorm:"column:stock" json:"stock"`

	BeginTime time.Time `gorm:"column:begin_time" json:"beginTime"`
	EndTime   time.Time `gorm:"column:end_time" json:"endTime"`

	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
}

func (SeckillVoucher) TableName() string {
	return "tb_seckill_voucher"
}

type VoucherOrder struct {
	ID        int64 `gorm:"column:id;primaryKey;autoIncrement:false" json:"id"`
	UserID    int64 `gorm:"column:user_id;uniqueIndex:uq_order_user_voucher" json:"userId"`
	VoucherID int64 `gorm:"column:voucher_id;uniqueIndex:uq_order_user_voucher" json:"voucherId"`
	PayType   int   `gorm:"column:pay_type" json:"payType"`
	Status    int   `gorm:"column:status" json:"status"`

	CreateTime time.Time  `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	PayTime    *time.Time `gorm:"column:pay_time" json:"payTime,omitempty"`
	UseTime    *time.Time `gorm:"column:use_time" json:"useTime,omitempty"`
	RefundTime *time.Time `gorm:"column:refund_time" json:"refundTime,omitempty"`
	UpdateTime time.Time  `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
}

func (VoucherOrder) TableName() string {
	return "tb_voucher_order"
}
