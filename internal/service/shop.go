package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/learning/go-dianping/internal/cache"
	"github.com/learning/go-dianping/internal/dto"
	"github.com/learning/go-dianping/internal/model"
	"github.com/learning/go-dianping/internal/repository"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var ErrInvalidShopCreat = errors.New("商铺参数错误")
var ErrInvalidShopQuery = errors.New("商铺查询参数错误")
var ErrGeoUnavailable = errors.New("GEO服务不可用")

type ShopService struct {
	repo      *repository.Repository
	rdb       *redis.Client
	cache     *cache.Client
	geoFlight singleflight.Group
}

func NewShopRepository(
	repo *repository.Repository,
	rdb *redis.Client,
) *ShopService {
	return &ShopService{
		repo:  repo,
		rdb:   rdb,
		cache: cache.New(rdb),
	}
}

func validateShop(shop *model.Shop) error {
	if strings.TrimSpace(shop.Name) == "" ||
		utf8.RuneCountInString(shop.Name) > 128 {
		return fmt.Errorf("%w：名称必填且最长128字", ErrInvalidShopCreat)
	}

	if shop.TypeID <= 0 {
		return fmt.Errorf("%w：商铺类型ID必须大于0", ErrInvalidShopCreat)
	}

	if strings.TrimSpace(shop.Address) == "" ||
		utf8.RuneCountInString(shop.Address) > 255 {
		return fmt.Errorf("%w：地址必填且最长255字", ErrInvalidShopCreat)
	}

	if utf8.RuneCountInString(shop.Images) > 1024 ||
		utf8.RuneCountInString(shop.Area) > 128 ||
		utf8.RuneCountInString(shop.OpenHours) > 32 {
		return fmt.Errorf("%w：图片、商圈或营业时间过长", ErrInvalidShopCreat)
	}

	if math.IsNaN(shop.X) || math.IsInf(shop.X, 0) ||
		math.IsNaN(shop.Y) || math.IsInf(shop.Y, 0) ||
		shop.X < -180 || shop.X > 180 ||
		shop.Y < -85.05112878 || shop.Y > 85.05112878 {
		return fmt.Errorf("%w：经纬度超出GEO支持范围", ErrInvalidShopCreat)
	}

	if shop.AvgPrice < 0 || shop.Sold < 0 ||
		shop.Comments < 0 || shop.Score < 0 || shop.Score > 50 {
		return fmt.Errorf(
			"%w：金额、销量、评论数不能为负，评分须为0到50",
			ErrInvalidShopCreat,
		)
	}

	return nil
}

func validCoordinates(x, y float64) bool {
	return !math.IsNaN(x) &&
		!math.IsNaN(y) &&
		!math.IsInf(x, 0) &&
		!math.IsInf(y, 0) &&
		x >= -180 &&
		x <= 180 &&
		y >= -85.05112878 &&
		y <= 85.05112878
}

func (s *ShopService) GetByID(
	ctx context.Context,
	id int64,
) (model.Shop, error) {
	var result model.Shop
	//每家商铺使用不同的缓存key
	key := "practice:shop:" + strconv.FormatInt(id, 10)
	//1.查Redis
	cached, err := s.rdb.Get(ctx, key).Result()
	if err == nil {
		//Redis保存的是JSON字符串，转回Shop
		if decodeErr := json.Unmarshal([]byte(cached), &result); decodeErr == nil {
			log.Printf("缓存命中：%s", key)
			return result, nil
		}
		log.Printf("缓存内容无法解析，重新查数据库：%s", key)
	} else if !errors.Is(err, redis.Nil) {
		// redis.Nil 表示 key 不存在，是正常的缓存未命中。
		// 其他错误表示 Redis 操作失败，记录后继续查数据库。
		log.Printf("读取缓存失败：%v", err)
	}
	//查数据库前，记住当前缓存版本
	keys := ShopKeys(id)
	version, versionErr := s.rdb.Get(ctx, keys.Version).Result()
	//版本key不存在是正常情况，用空字符串代表
	if errors.Is(versionErr, redis.Nil) {
		version = ""
		versionErr = nil
	}
	if versionErr != nil {
		log.Printf("读取商铺缓存版本失败：%v", versionErr)
	}
	// 2. 没拿到有效缓存，查 MySQL。
	log.Printf("查询 MySQL：id=%d", id)
	result, err = s.repo.FindById(ctx, id)
	if err != nil {
		return model.Shop{}, err
	}

	//3.把商铺转成JSON，写入Redis，保存5分钟
	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("商铺序列化失败：%v", err)
		return result, nil
	}
	if versionErr == nil {
		stored, cacheErr := s.cache.SetIfVersion(
			ctx,
			keys,
			version,
			data,
			5*time.Minute,
		)
		if cacheErr != nil {
			log.Printf("写入商铺缓存失败：%v", cacheErr)
		} else if !stored {
			log.Printf("商铺缓存版本已变化，跳过回填：id=%d", id)
		}
	}
	//Mysql已经查成功，缓存写入失败也可以返回商铺
	return result, nil
}

func (s *ShopService) ShopCreate(ctx context.Context, shop *model.Shop) (int64, error) {
	if shop == nil {
		return 0, ErrInvalidShopCreat
	}
	if err := validateShop(shop); err != nil {
		return 0, err
	}
	exists, err := s.repo.ShopTypeExists(ctx, shop.TypeID)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, fmt.Errorf("%w：商铺类型不存在", ErrInvalidShopCreat)
	}
	shop.Id = 0
	shop.CreateTime = time.Time{}
	shop.UpdateTime = time.Time{}
	shop.Distance = nil
	if err = s.repo.ShopCreate(ctx, shop); err != nil {
		return 0, err
	}

	if err = s.SyncGeo(ctx, nil, shop); err != nil {
		s.geoFailed(ctx, err)
	}

	if err = s.cache.Invalidate(ctx, ShopKeys(shop.Id)); err != nil {
		log.Printf("商铺已创建，但缓存失效处理失败：%v", err)
	}
	return shop.Id, nil
}

func (s *ShopService) SyncGeo(
	ctx context.Context,
	old, next *model.Shop,
) error {
	pipe := s.rdb.TxPipeline()
	if old != nil && old.TypeID != next.TypeID {
		oldKey := "practice:shop:geo:" + strconv.FormatInt(old.TypeID, 10)
		pipe.ZRem(ctx, oldKey, strconv.FormatInt(old.Id, 10))
	}
	// 向当前分类写入商铺位置。
	key := "practice:shop:geo:" +
		strconv.FormatInt(next.TypeID, 10)

	pipe.GeoAdd(ctx, key, &redis.GeoLocation{
		Name:      strconv.FormatInt(next.Id, 10),
		Longitude: next.X,
		Latitude:  next.Y,
	})

	_, err := pipe.Exec(ctx)
	return err
}

func (s *ShopService) WarmGeo(ctx context.Context) error {
	shops, err := s.repo.ShopAll(ctx)
	if err != nil {
		return err
	}
	groups := make(map[int64][]*redis.GeoLocation)
	for _, shop := range shops {
		if !validCoordinates(shop.X, shop.Y) {
			return fmt.Errorf(
				"商铺%d的经纬度无效",
				shop.Id,
			)
		}
		location := &redis.GeoLocation{
			Name:      strconv.FormatInt(shop.Id, 10),
			Longitude: shop.X,
			Latitude:  shop.Y,
		}
		groups[shop.TypeID] = append(groups[shop.TypeID], location)
	}
	// 使用 Pipeline 批量写入 Redis
	pipe := s.rdb.TxPipeline()

	for typeID, locations := range groups {
		key := "practice:shop:geo:" +
			strconv.FormatInt(typeID, 10)

		pipe.GeoAdd(ctx, key, locations...)
	}

	// 标记 GEO 已完成预热
	pipe.Set(
		ctx,
		shopGeoReadyKey,
		"1",
		10*time.Minute,
	)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf(
			"%w: GEO索引写入失败: %v",
			ErrGeoUnavailable,
			err,
		)
	}

	return nil
}

const shopGeoReadyKey = "practice:shop:geo:ready"

func (s *ShopService) geoFailed(ctx context.Context, cause error) {
	log.Printf("同步商铺GEO失败:%v", cause)
	if err := s.rdb.Del(ctx, shopGeoReadyKey).Err(); err != nil {
		log.Printf("清除GEO就绪标记失败:%v", err)
	}
}

func ShopKeys(id int64) cache.Keys {
	suffix := strconv.FormatInt(id, 10)
	return cache.Keys{
		Data:    "practice:shop:" + suffix,
		Lock:    "practice:shop:lock:" + suffix,
		Version: "practice:shop:version:" + suffix,
	}
}

func (s *ShopService) ShopUpdate(ctx context.Context, in dto.ShopInput) error {
	if in.ID <= 0 || !in.HasChanges() {
		fmt.Print('1')
		return fmt.Errorf("%w：原因", ErrInvalidShopCreat)
	}
	old, err := s.repo.FindById(ctx, in.ID)
	if err != nil {
		return fmt.Errorf("%w：原因", ErrInvalidShopCreat)
	}
	next := old
	in.ApplyTo(&next)
	if err = validateShop(&next); err != nil {
		return err
	}
	if next.TypeID != old.TypeID {
		exists, err := s.repo.ShopTypeExists(ctx, next.TypeID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w：商铺类型不存在", ErrInvalidShopCreat)
		}
	}
	if err = s.repo.ShopChange(ctx, in); err != nil {
		return err
	}
	if err = s.cache.Invalidate(ctx, ShopKeys(in.ID)); err != nil {
		log.Printf("商铺已更新，但缓存失效处理失败：%v", err)
	}
	if err = s.SyncGeo(ctx, &old, &next); err != nil {
		s.geoFailed(ctx, err)
	}
	return nil
}

func (s *ShopService) ListByType(
	ctx context.Context,
	typeID int64,
	current int,
	x, y *float64,
) ([]model.Shop, error) {
	if typeID <= 0 {
		return nil, fmt.Errorf(
			"%w: 商铺类型ID必须大于0",
			ErrInvalidShopQuery,
		)
	}
	if current < 1 {
		return nil, fmt.Errorf(
			"%w: 页码必须大于0",
			ErrInvalidShopQuery,
		)
	}
	if x == nil && y == nil {
		return s.repo.ShopListByType(
			ctx,
			typeID,
			current,
			5,
		)
	}

	if x == nil || y == nil {
		return nil, fmt.Errorf(
			"%w: x和y必须同时提供",
			ErrInvalidShopQuery,
		)
	}
	if !validCoordinates(*x, *y) {
		return nil, fmt.Errorf(
			"%w: x和y超出经纬度范围",
			ErrInvalidShopQuery,
		)
	}
	ready, err := s.rdb.Exists(ctx, shopGeoReadyKey).Result()
	if err != nil {
		return nil, fmt.Errorf(
			"%w: 检查GEO索引状态失败: %v",
			ErrGeoUnavailable,
			err,
		)
	}
	if ready == 0 {
		_, err, _ := s.geoFlight.Do(
			shopGeoReadyKey,
			func() (any, error) {
				return nil, s.WarmGeo(ctx)
			},
		)
		if err != nil {
			return nil, fmt.Errorf(
				"GEO索引重建失败:%w",
				err,
			)
		}
	}
	from := (current - 1) * 5
	to := current * 5
	// 按商铺类型建立 GEO key
	geoKey := "practice:shop:geo:" +
		strconv.FormatInt(typeID, 10)

	items, err := s.rdb.GeoSearchLocation(
		ctx,
		geoKey,
		&redis.GeoSearchLocationQuery{
			GeoSearchQuery: redis.GeoSearchQuery{
				Longitude:  *x,
				Latitude:   *y,
				Radius:     5000,
				RadiusUnit: "m",
				Sort:       "ASC",
				Count:      to,
			},
			WithDist: true,
		},
	).Result()
	if err != nil {
		return nil, fmt.Errorf(
			"%w: GEO查询失败: %v",
			ErrGeoUnavailable,
			err,
		)
	}
	if len(items) <= from {
		return []model.Shop{}, nil
	}

	// 截取当前页
	items = items[from:]

	// 先把 Redis 返回的商铺 ID 解析出来
	ids := make([]int64, 0, len(items))

	for _, item := range items {
		id, err := strconv.ParseInt(item.Name, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Redis GEO中的商铺ID无效: %w", err)
		}

		ids = append(ids, id)
	}
	// 根据 Redis GEO 返回的 ID，批量查询 MySQL
	shops, err := s.repo.ShopListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	// MySQL 返回的顺序不一定和 ids 一样。
	// 先转换成 map，方便根据 ID 快速查找。
	shopByID := make(map[int64]model.Shop, len(shops))

	for _, shop := range shops {
		shopByID[shop.Id] = shop
	}

	// 按 Redis GEO 返回的顺序重新组装结果
	ordered := make([]model.Shop, 0, len(ids))

	for i, id := range ids {
		shop, exists := shopByID[id]
		if !exists {
			// Redis 中有这个 ID，但数据库已经没有了，跳过它。
			continue
		}

		// GEO key 按类型保存，正常情况下这里一定相等。
		// 这个判断可以避免脏数据混入结果。
		if shop.TypeID != typeID {
			continue
		}

		// items[i] 和 ids[i] 一一对应
		distance := items[i].Dist
		shop.Distance = &distance

		ordered = append(ordered, shop)
	}

	return ordered, nil
}
