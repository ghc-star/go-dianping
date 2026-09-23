package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/learning/go-dianping/internal/dto"
	"github.com/learning/go-dianping/internal/model"
	"github.com/learning/go-dianping/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type BlogService struct {
	repo *repository.Repository
	rdb  *redis.Client
}

func NewBlogService(
	repo *repository.Repository,
	rdb *redis.Client,
) *BlogService {
	return &BlogService{
		repo: repo,
		rdb:  rdb,
	}
}

var ErrInvalidBlog = errors.New("笔记参数错误")
var ErrBlogLikeBusy = errors.New("点赞操作繁忙")

const (
	blogLikesCacheTTL = 5 * time.Minute
	blogLikesLockTTL  = 10 * time.Second
	blogLikesLockWait = 500 * time.Millisecond
)

var unlockBlogLikesScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

func blogLikesKey(blogID int64) string {
	return fmt.Sprintf("blog:liked:%d", blogID)
}

func blogLikesExistsKey(blogID int64) string {
	return fmt.Sprintf("blog:liked:%d:exists", blogID)
}

func blogLikesLockKey(blogID int64) string {
	return fmt.Sprintf("lock:blog:liked:%d", blogID)
}

func newBlogLikesLockToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func validateBlogCreate(userID int64, input dto.BlogCreateRequest) error {
	if userID <= 0 {
		return fmt.Errorf("%w：用户ID无效", ErrInvalidBlog)
	}

	if input.ShopID <= 0 {
		return fmt.Errorf("%w：商铺ID必须大于0", ErrInvalidBlog)
	}

	title := strings.TrimSpace(input.Title)
	if title == "" || utf8.RuneCountInString(title) > 255 {
		return fmt.Errorf(
			"%w：标题必填且不能超过255个字符",
			ErrInvalidBlog,
		)
	}

	content := strings.TrimSpace(input.Content)
	if content == "" || utf8.RuneCountInString(content) > 2048 {
		return fmt.Errorf(
			"%w：正文必填且不能超过2048个字符",
			ErrInvalidBlog,
		)
	}

	if utf8.RuneCountInString(input.Images) > 2048 {
		return fmt.Errorf(
			"%w：图片地址总长度不能超过2048个字符",
			ErrInvalidBlog,
		)
	}

	images := strings.Split(input.Images, ",")
	if len(images) > 9 {
		return fmt.Errorf("%w：图片最多9张", ErrInvalidBlog)
	}

	for _, image := range images {
		if strings.TrimSpace(image) == "" {
			return fmt.Errorf(
				"%w：至少上传一张图片且图片地址不能为空",
				ErrInvalidBlog,
			)
		}
	}

	return nil
}

func (s *BlogService) Create(
	ctx context.Context,
	userID int64,
	input dto.BlogCreateRequest,
) (int64, error) {
	if err := validateBlogCreate(userID, input); err != nil {
		return 0, err
	}
	_, err := s.repo.FindById(ctx, input.ShopID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, gorm.ErrRecordNotFound
	}
	if err != nil {
		return 0, err
	}
	blog := model.Blog{
		ShopID:   input.ShopID,
		UserID:   userID,
		Title:    strings.TrimSpace(input.Title),
		Images:   input.Images,
		Content:  input.Content,
		Liked:    0,
		Comments: 0,
	}
	if err := s.repo.CreateBlogWithFeed(ctx, &blog); err != nil {
		return 0, err
	}

	if _, err := s.deliverFeedBatch(ctx); err != nil {
		log.Printf(
			"笔记已发布，Feed将在稍后重试，blogID=%d：%v",
			blog.ID,
			err,
		)
	}

	return blog.ID, nil

}

func (s *BlogService) GetByID(
	ctx context.Context,
	id int64,
) (model.Blog, error) {
	if id <= 0 {
		return model.Blog{}, fmt.Errorf(
			"%w:笔记ID必须大于0",
			ErrInvalidBlog,
		)
	}
	blog, err := s.repo.FindBlogByID(ctx, id)
	if err != nil {
		return model.Blog{}, err
	}

	user, err := s.repo.FindUser(ctx, blog.UserID)
	if err != nil {
		return model.Blog{}, err
	}
	blog.Name = user.NickName
	blog.Icon = user.Icon

	return blog, nil
}

func (s *BlogService) ListByUser(
	ctx context.Context,
	userID int64,
	current int,
) ([]model.Blog, error) {
	if userID <= 0 {
		return nil, fmt.Errorf(
			"%w：用户ID必须大于0",
			ErrInvalidBlog,
		)
	}

	if current <= 0 {
		return nil, fmt.Errorf(
			"%w：页码必须大于0",
			ErrInvalidBlog,
		)
	}
	if _, err := s.repo.FindUser(ctx, userID); err != nil {
		return nil, err
	}

	return s.repo.FindBlogsByUserID(
		ctx,
		userID,
		current,
		10,
	)
}

func (s *BlogService) ListHot(
	ctx context.Context,
	current int,

) ([]model.Blog, error) {

	if current <= 0 {
		return nil, fmt.Errorf(
			"%w：页码必须大于0",
			ErrInvalidBlog,
		)
	}
	blogs, err := s.repo.FindHotBlogs(ctx, current, 10)
	if err != nil {
		return nil, err
	}
	if len(blogs) == 0 {
		return blogs, nil
	}
	userIDs := make([]int64, 0, len(blogs))
	seen := make(map[int64]struct{}, len(blogs))
	for _, blog := range blogs {
		userID := blog.UserID
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}
	users, err := s.repo.FindUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	usersByID := make(map[int64]model.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}
	for i := range blogs {
		user, exists := usersByID[blogs[i].UserID]
		if !exists {
			return nil, gorm.ErrRecordNotFound
		}
		blogs[i].Name = user.NickName
		blogs[i].Icon = user.Icon
	}

	return blogs, nil
}

func (s *BlogService) ToggleLike(
	ctx context.Context,
	userID int64,
	blogID int64,
) error {
	if userID <= 0 {
		return fmt.Errorf(
			"%w：用户ID必须大于0",
			ErrInvalidBlog,
		)
	}
	if blogID <= 0 {
		return fmt.Errorf(
			"%w：笔记ID必须大于0",
			ErrInvalidBlog,
		)
	}

	token, acquired, lockErr := s.acquireBlogLikesLock(ctx, blogID)
	if lockErr != nil {
		// Redis 故障不能阻止 MySQL 中的点赞关系发生变更。
		_, err := s.repo.ToggleBlogLike(ctx, blogID, userID)
		return err
	}
	if !acquired {
		return ErrBlogLikeBusy
	}
	defer s.releaseBlogLikesLock(blogID, token)

	if _, err := s.repo.ToggleBlogLike(ctx, blogID, userID); err != nil {
		return err
	}

	if err := s.rdb.Del(
		ctx,
		blogLikesKey(blogID),
		blogLikesExistsKey(blogID),
	).Err(); err != nil {
		// MySQL 已经提交，缓存失效失败不能让客户端误以为点赞失败。
		log.Printf("删除点赞缓存失败，blogID=%d：%v", blogID, err)
	}
	return nil
}

func (s *BlogService) ListEarliestLikers(
	ctx context.Context,
	blogID int64,
) ([]dto.UserDTO, error) {
	if blogID <= 0 {
		return nil, fmt.Errorf(
			"%w：笔记ID必须大于0",
			ErrInvalidBlog,
		)
	}
	_, err := s.repo.FindBlogByID(ctx, blogID)
	if err != nil {
		return nil, err
	}
	if userIDs, hit, cacheErr := s.readBlogLikesCache(ctx, blogID); cacheErr == nil && hit {
		return s.findLikeUsers(ctx, userIDs)
	}

	token, acquired, lockErr := s.acquireBlogLikesLock(ctx, blogID)
	if lockErr != nil || !acquired {
		return s.loadEarliestLikersFromDB(ctx, blogID, false)
	}
	defer s.releaseBlogLikesLock(blogID, token)

	// 等锁期间其他请求可能已经完成回填，所以需要再次检查。
	if userIDs, hit, cacheErr := s.readBlogLikesCache(ctx, blogID); cacheErr == nil && hit {
		return s.findLikeUsers(ctx, userIDs)
	}

	return s.loadEarliestLikersFromDB(ctx, blogID, true)
}

func (s *BlogService) readBlogLikesCache(
	ctx context.Context,
	blogID int64,
) ([]int64, bool, error) {
	exists, err := s.rdb.Exists(ctx, blogLikesExistsKey(blogID)).Result()
	if err != nil || exists == 0 {
		return nil, false, err
	}

	members, err := s.rdb.ZRange(ctx, blogLikesKey(blogID), 0, 4).Result()
	if err != nil {
		return nil, false, err
	}
	userIDs := make([]int64, 0, len(members))
	for _, member := range members {
		userID, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			return nil, false, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, true, nil
}

func (s *BlogService) loadEarliestLikersFromDB(
	ctx context.Context,
	blogID int64,
	cacheResult bool,
) ([]dto.UserDTO, error) {
	likes, err := s.repo.FindEarliestBlogLikes(ctx, blogID, 5)
	if err != nil {
		return nil, err
	}

	userIDs := make([]int64, 0, len(likes))
	members := make([]redis.Z, 0, len(likes))
	for _, like := range likes {
		userIDs = append(userIDs, like.UserID)
		members = append(members, redis.Z{
			Score:  float64(like.CreateTime.UnixMilli()),
			Member: strconv.FormatInt(like.UserID, 10),
		})
	}

	if cacheResult {
		pipe := s.rdb.TxPipeline()
		pipe.Del(ctx, blogLikesKey(blogID))
		if len(members) > 0 {
			pipe.ZAdd(ctx, blogLikesKey(blogID), members...)
			pipe.Expire(ctx, blogLikesKey(blogID), blogLikesCacheTTL)
		}
		pipe.Set(
			ctx,
			blogLikesExistsKey(blogID),
			"1",
			blogLikesCacheTTL,
		)
		if _, err := pipe.Exec(ctx); err != nil {
			log.Printf("回填点赞缓存失败，blogID=%d：%v", blogID, err)
		}
	}

	return s.findLikeUsers(ctx, userIDs)
}

func (s *BlogService) findLikeUsers(
	ctx context.Context,
	userIDs []int64,
) ([]dto.UserDTO, error) {
	if len(userIDs) == 0 {
		return []dto.UserDTO{}, nil
	}
	users, err := s.repo.FindUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	usersByID := make(map[int64]model.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}
	result := make([]dto.UserDTO, 0, len(userIDs))
	for _, userID := range userIDs {
		user, exists := usersByID[userID]
		if !exists {
			return nil, gorm.ErrRecordNotFound
		}

		result = append(result, dto.UserDTO{
			ID:       user.ID,
			NickName: user.NickName,
			Icon:     user.Icon,
		})
	}

	return result, nil
}

func (s *BlogService) acquireBlogLikesLock(
	ctx context.Context,
	blogID int64,
) (string, bool, error) {
	token, err := newBlogLikesLockToken()
	if err != nil {
		return "", false, err
	}

	deadline := time.Now().Add(blogLikesLockWait)
	for {
		acquired, err := s.rdb.SetNX(
			ctx,
			blogLikesLockKey(blogID),
			token,
			blogLikesLockTTL,
		).Result()
		if err != nil {
			return "", false, err
		}
		if acquired {
			return token, true, nil
		}
		if time.Now().After(deadline) {
			return "", false, nil
		}

		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-time.After(30 * time.Millisecond):
		}
	}
}

func (s *BlogService) releaseBlogLikesLock(blogID int64, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := unlockBlogLikesScript.Run(
		ctx,
		s.rdb,
		[]string{blogLikesLockKey(blogID)},
		token,
	).Err(); err != nil {
		log.Printf("释放点赞缓存锁失败，blogID=%d：%v", blogID, err)
	}
}

func feedKey(userID int64) string {
	return "feed:" + strconv.FormatInt(userID, 10)
}

const feedPageSize = 2

func nextFeedCursor(
	items []redis.Z,
	max int64,
	offset int,
) (int64, int) {
	if len(items) == 0 {
		return max, offset
	}
	minTime := int64(items[len(items)-1].Score)
	sameTimeCount := 0
	for i := len(items) - 1; i >= 0; i-- {
		if int64(items[i].Score) != minTime {
			break
		}
		sameTimeCount++
	}
	nextOffset := sameTimeCount
	if minTime == max {
		nextOffset += offset
	}
	return minTime, nextOffset
}

func (s *BlogService) Feed(
	ctx context.Context,
	userID int64,
	max int64,
	offset int,
) (*dto.ScrollResult, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w:用户ID必须大于0", ErrInvalidBlog)
	}
	if max < 0 || offset < 0 {
		return nil, fmt.Errorf(
			"%w：lastId和offset不能为负数",
			ErrInvalidBlog,
		)
	}

	if err := s.ensureFeedInbox(ctx, userID); err != nil {
		return nil, err
	}

	items, err := s.rdb.ZRevRangeByScoreWithScores(
		ctx,
		feedKey(userID),
		&redis.ZRangeBy{
			Min:    "0",
			Max:    strconv.FormatInt(max, 10),
			Offset: int64(offset),
			Count:  feedPageSize,
		},
	).Result()
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return &dto.ScrollResult{
			List:    []model.Blog{},
			MinTime: max,
			Offset:  offset,
		}, nil
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		id, err := strconv.ParseInt(fmt.Sprint(item.Member), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Feed中的笔记ID格式错误：%w", err)
		}
		ids = append(ids, id)
	}
	blogs, err := s.repo.FindBlogsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	blogsByID := make(map[int64]model.Blog, len(blogs))
	for _, blog := range blogs {
		blogsByID[blog.ID] = blog
	}
	ordered := make([]model.Blog, 0, len(ids))
	for _, id := range ids {
		blog, exists := blogsByID[id]
		if exists {
			ordered = append(ordered, blog)
		}
	}

	if err := s.fillBlogAuthors(ctx, ordered); err != nil {
		return nil, err
	}

	minTime, nextOffset := nextFeedCursor(items, max, offset)

	return &dto.ScrollResult{
		List:    ordered,
		MinTime: minTime,
		Offset:  nextOffset,
	}, nil

}

func (s *BlogService) fillBlogAuthors(
	ctx context.Context,
	blogs []model.Blog,
) error {
	if len(blogs) == 0 {
		return nil
	}

	userIDs := make([]int64, 0, len(blogs))
	seen := make(map[int64]struct{}, len(blogs))

	for _, blog := range blogs {
		if _, exists := seen[blog.UserID]; exists {
			continue
		}
		seen[blog.UserID] = struct{}{}
		userIDs = append(userIDs, blog.UserID)
	}

	users, err := s.repo.FindUsersByIDs(ctx, userIDs)
	if err != nil {
		return err
	}

	usersByID := make(map[int64]model.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	for i := range blogs {
		user, exists := usersByID[blogs[i].UserID]
		if !exists {
			return gorm.ErrRecordNotFound
		}
		blogs[i].Name = user.NickName
		blogs[i].Icon = user.Icon
	}

	return nil
}

func (s *BlogService) deliverFeedBatch(
	ctx context.Context,
) (int, error) {
	rows, err := s.repo.FindPendingFeedOutbox(ctx, 100)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	pipe := s.rdb.Pipeline()
	for _, row := range rows {
		pipe.ZAdd(
			ctx,
			feedKey(row.UserID),
			redis.Z{
				Score:  float64(row.Score),
				Member: strconv.FormatInt(row.BlogID, 10),
			},
		)
	}

	readyKeys := make([]string, 0, len(rows))
	seen := make(map[int64]struct{}, len(rows))

	for _, row := range rows {
		if _, exists := seen[row.UserID]; exists {
			continue
		}

		seen[row.UserID] = struct{}{}
		readyKeys = append(readyKeys, feedReadyKey(row.UserID))
	}

	for _, key := range readyKeys {
		pipe.Set(ctx, key, "1", 24*time.Hour)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		// Redis 超时或断线时不能标记 delivered_at，下次按 ZADD 幂等重投。
		return 0, err
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	if err := s.repo.MarkFeedOutboxDelivered(ctx, ids); err != nil {
		// MySQL 失败时 Redis 可能已写入。ZADD 幂等，不能当成投递成功。
		return 0, err
	}

	return len(rows), nil
}

func (s *BlogService) StartFeedWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := s.deliverFeedBatch(ctx)
			if err != nil {
				log.Printf("Feed后台投递失败：%v", err)
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if backoff < 8*time.Second {
					backoff *= 2
				}
				continue
			}
			backoff = time.Second
			if count > 0 {
				log.Printf("Feed后台投递完成,数量=%d", count)
			}
		}
	}
}

func feedReadyKey(userID int64) string {
	return "feed:ready:" + strconv.FormatInt(userID, 10)
}

func (s *BlogService) ensureFeedInbox(
	ctx context.Context,
	userID int64,
) error {
	ready, err := s.rdb.Exists(
		ctx,
		feedReadyKey(userID),
	).Result()

	if err != nil {
		return err
	}

	if ready > 0 {
		return nil
	}

	rows, err := s.repo.FindFeedHistory(ctx, userID)
	if err != nil {
		return err
	}

	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, feedKey(userID))
	if len(rows) > 0 {
		members := make([]redis.Z, 0, len(rows))

		for _, row := range rows {
			members = append(members, redis.Z{
				Score:  float64(row.Score),
				Member: strconv.FormatInt(row.BlogID, 10),
			})
		}

		pipe.ZAdd(ctx, feedKey(userID), members...)
		pipe.Expire(ctx, feedKey(userID), 24*time.Hour)
	}

	// 即使用户没有任何 Feed，也要记录 ready，避免每次都查数据库。
	pipe.Set(
		ctx,
		feedReadyKey(userID),
		"1",
		24*time.Hour,
	)

	_, err = pipe.Exec(ctx)
	return err
}
