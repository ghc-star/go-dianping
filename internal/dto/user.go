package dto

type LoginRequest struct {
	Phone    string `json:"phone" binding:"required"`
	Code     string `json:"code" binding:"required,len=6,numeric"`
	Password string `json:"password"`
}
type Password struct {
	Phone    string `json:"phone" binding:"required"`
	Password string `json:"password"`
}

type UserDTO struct {
	ID       int64  `json:"id"`
	NickName string `json:"nickName"`
	Icon     string `json:"icon"`
}

type UpdateUserInfoRequest struct {
	City      *string `json:"city"`
	Introduce *string `json:"introduce"`
	Birthday  *string `json:"birthday"`
}

type FollowRequest struct {
	FollowUserID int64 `json:"followUserId" binding:"required,gt=0"`
	IsFollow     *bool `json:"isFollow" binding:"required"`
}

type FollowTargetRequest struct {
	FollowUserID int64 `json:"followUserId" binding:"required,gt=0"`
}
