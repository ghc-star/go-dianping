package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/learning/go-dianping/internal/middleware"
	"github.com/learning/go-dianping/internal/service"
)

func RegisterRoutes(
	r *gin.Engine,
	shopHandler *ShopHandler,
	UserHandler *UserHandler,
	blogHandler *BlogHandler,
	followHandler *FollowHandler,
	uploadHandler *UploadHandler,
	voucherHandler *VoucherHandler,
	userService *service.UserService,
) {

	r.GET("/shop/of/type", shopHandler.ListByType)
	r.GET("/shop/:id", shopHandler.GetByID)
	r.POST("/user/code", UserHandler.SendCode)
	r.POST("/user/login", UserHandler.Login)
	r.GET("/blog/hot", blogHandler.ListHot)
	r.GET("/voucher/list/:shopId", voucherHandler.ListByShop)
	auth := r.Group("", middleware.RequireLogin(userService))
	auth.GET("/user/me", UserHandler.Me)
	auth.POST("/user/logout", UserHandler.Logout)
	auth.GET("/user/:id", UserHandler.FindUser)
	auth.PUT("/user/info", UserHandler.UpdateUserInfo)
	auth.GET("/user/info/:id", UserHandler.UserInfo)
	auth.PUT("/user/name", UserHandler.SaveUserName)
	auth.POST("/user/password", UserHandler.SetPassword)
	auth.POST("/user/sign", UserHandler.Sign)
	auth.GET("/user/sign/count", UserHandler.SignCount)
	auth.POST("/shop", shopHandler.ShopCreate)
	auth.PUT("/shop", shopHandler.ShopUpdate)
	auth.POST("/blog", blogHandler.Create)
	auth.GET("/blog/of/me", blogHandler.ListMine)
	auth.GET("/blog/of/user", blogHandler.ListByUser)
	auth.GET("/blog/likes/:id", blogHandler.ListEarliestLikers)
	auth.PUT("/blog/like/:id", blogHandler.ToggleLike)
	auth.GET("/blog/:id", blogHandler.GetByID)
	auth.PUT("/follow", followHandler.Follow)
	auth.POST("/follow/status", followHandler.IsFollow)
	auth.POST("/follow/common", followHandler.CommonFollows)
	auth.GET("/blog/of/follow", blogHandler.Feed)

	auth.POST("/upload/blog", uploadHandler.UploadBlog)
	auth.DELETE("/upload/blog/delete", uploadHandler.DeleteBlog)

	auth.POST("/voucher", voucherHandler.Create)
	auth.POST("/voucher/seckill", voucherHandler.CreateSeckill)
	auth.POST(
		"/voucher-order/seckill/:id",
		voucherHandler.Seckill,
	)
	auth.GET(
		"/voucher-order/status/:id",
		voucherHandler.FindOrderStatus,
	)
	auth.GET("/voucher-order/:id", voucherHandler.FindOrder)
}
