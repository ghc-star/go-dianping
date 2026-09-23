package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/learning/go-dianping/internal/handler"
	"github.com/learning/go-dianping/internal/mq"
	"github.com/learning/go-dianping/internal/repository"
	"github.com/learning/go-dianping/internal/service"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./data/uploads"
	}

	uploadMaxBytes := int64(5 * 1024 * 1024)

	uploadMaxBytesText, configured := os.LookupEnv("UPLOAD_MAX_BYTES")
	if configured {
		value, err := strconv.ParseInt(uploadMaxBytesText, 10, 64)
		if err != nil || value <= 0 {
			log.Fatal("UPLOAD_MAX_BYTES必须是正整数")
		}
		uploadMaxBytes = value
	}

	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("请设置MYSQL_DSN环境变量")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("连接数据库失败:", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("获取数据库连接池失败:", err)
	}
	shopRepo := repository.New(db)
	defer sqlDB.Close()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		Password:     os.Getenv("REDIS_PASSWORD"),
		DB:           0,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolTimeout:  4 * time.Second,
	})
	defer rdb.Close()
	//启动时检查连接，最多等3秒
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = rdb.Ping(ctx).Err()
	cancel()
	if err != nil {
		log.Fatal("连接redis失败:", err)
	}

	shopService := service.NewShopRepository(shopRepo, rdb)
	// 启动时将 MySQL 中的商铺预热到 Redis GEO
	warmCtx, warmCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)

	if err := shopService.WarmGeo(warmCtx); err != nil {
		warmCancel()
		log.Fatal("商铺 GEO 预热失败:", err)
	}

	warmCancel()
	shopHandler := handler.NewShopHandler(shopService)
	userService := service.NewUservice(shopRepo, rdb)
	userHandler := handler.NewUserHandler(userService)
	blogService := service.NewBlogService(shopRepo, rdb)
	voucherService := service.NewVoucherService(shopRepo, rdb)
	var rabbit *mq.Client
	if rabbitURL := os.Getenv("RABBITMQ_URL"); rabbitURL != "" {
		rabbit, err = mq.Dial(rabbitURL)
		if err != nil {
			log.Fatal("连接RabbitMQ失败:", err)
		}
		defer rabbit.Close()
		voucherService.ConfigureRabbit(rabbit)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		blogService.StartFeedWorker(workerCtx)
	}()
	go func() {
		defer wg.Done()
		voucherService.StartSeckillWorker(workerCtx)
	}()

	blogHandler := handler.NewBlogHandler(blogService)

	followService := service.NewFollowService(shopRepo, rdb)
	followHandler := handler.NewFollowHandler(followService)
	uploadHandler := handler.NewUploadHandler(uploadDir, uploadMaxBytes)

	voucherHandler := handler.NewVoucherHandler(voucherService)
	r := gin.Default()
	r.Static("/imgs", uploadDir)
	handler.RegisterRoutes(
		r,
		shopHandler,
		userHandler,
		blogHandler,
		followHandler,
		uploadHandler,
		voucherHandler,
		userService,
	)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil &&
			err != http.ErrServerClosed {
			log.Fatal("HTTP 服务异常退出:", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 关闭：%v", err)
	}

	workerCancel()
	wg.Wait()
}
