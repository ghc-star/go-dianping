package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/learning/go-dianping/internal/dto"
	"github.com/learning/go-dianping/internal/model"
	"github.com/learning/go-dianping/internal/repository"
	"github.com/redis/go-redis/v9"
)

var ErrInvalidFollow = errors.New("关注参数错误")

const (
	followCacheTTL      = 31 * time.Minute
	followReadyCacheTTL = 30 * time.Minute
)

type FollowService struct {
	repo *repository.Repository
	rdb  *redis.Client
}

func NewFollowService(
	repo *repository.Repository,
	rdb *redis.Client,
) *FollowService {
	return &FollowService{
		repo: repo,
		rdb:  rdb,
	}
}
func followKey(userID int64) string {
	return "follows:" + strconv.FormatInt(userID, 10)
}

func followReadyKey(userID int64) string {
	return "follows:ready:" + strconv.FormatInt(userID, 10)
}

func (s *FollowService) Follow(
	ctx context.Context,
	userID int64,
	followUserID int64,
	isFollow bool,
) error {
	if userID <= 0 || followUserID <= 0 {
		return fmt.Errorf("%w:用户ID必须大于0", ErrInvalidFollow)
	}
	if userID == followUserID {
		return fmt.Errorf("%w:不能关注自己", ErrInvalidFollow)
	}
	if _, err := s.repo.FindUser(ctx, followUserID); err != nil {
		return err
	}
	if err := s.repo.SetFollow(
		ctx,
		userID,
		followUserID,
		isFollow,
	); err != nil {
		return err
	}

	if err := s.rdb.Del(
		ctx,
		followKey(userID),
		followReadyKey(userID),
	).Err(); err != nil {
		log.Printf(
			"删除关注缓存失败，userID=%d:%v",
			userID,
			err,
		)
	}
	return nil
}

func (s *FollowService) IsFollow(
	ctx context.Context,
	userID int64,
	followUserID int64,
) (bool, error) {
	if userID <= 0 || followUserID <= 0 {
		return false, fmt.Errorf(
			"%w:用户ID必须大于0",
			ErrInvalidFollow,
		)
	}
	if _, err := s.repo.FindUser(ctx, followUserID); err != nil {
		return false, err
	}
	return s.repo.IsFollow(ctx, userID, followUserID)
}

func (s *FollowService) CommonFollows(
	ctx context.Context,
	userID int64,
	targetUserID int64,
) ([]dto.UserDTO, error) {
	if userID <= 0 || targetUserID <= 0 {
		return nil, fmt.Errorf(
			"%w：用户ID必须大于0",
			ErrInvalidFollow,
		)
	}
	if _, err := s.repo.FindUser(ctx, targetUserID); err != nil {
		return nil, err
	}

	redisAvailable := true
	if err := s.ensureFollowSet(ctx, userID); err != nil {
		log.Printf("加载当前用户关注缓存失败:%v", err)
		redisAvailable = false
	}
	if err := s.ensureFollowSet(ctx, targetUserID); err != nil {
		log.Printf("加载目标用户关注缓存失败：%v", err)
		redisAvailable = false
	}

	var ids []int64

	if redisAvailable {
		members, err := s.rdb.SInter(
			ctx,
			followKey(userID),
			followKey(targetUserID),
		).Result()
		if err != nil {
			log.Printf("Redis计算共同关注失败：%v", err)
			redisAvailable = false
		} else {
			ids = make([]int64, 0, len(members))

			for _, member := range members {
				id, err := strconv.ParseInt(member, 10, 64)
				if err != nil {
					log.Printf("关注缓存成员格式错误：%q", member)
					redisAvailable = false
					break
				}
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool {
				return ids[i] < ids[j]
			})
		}
	}
	if !redisAvailable {
		var err error
		ids, err = s.repo.FindCommonFollowUserIDs(
			ctx,
			userID,
			targetUserID,
		)
		if err != nil {
			return nil, err
		}
	}

	if len(ids) == 0 {
		return []dto.UserDTO{}, nil
	}
	users, err := s.repo.FindUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	usersByID := make(map[int64]model.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	result := make([]dto.UserDTO, 0, len(ids))

	for _, id := range ids {
		user, ok := usersByID[id]
		if !ok {
			continue
		}

		result = append(result, dto.UserDTO{
			ID:       user.ID,
			NickName: user.NickName,
			Icon:     user.Icon,
		})
	}

	return result, nil
}

func (s *FollowService) ensureFollowSet(
	ctx context.Context,
	userID int64,
) error {
	ready, err := s.rdb.Exists(
		ctx,
		followReadyKey(userID),
	).Result()
	if err != nil {
		return err
	}

	if ready > 0 {
		return nil
	}
	ids, err := s.repo.FindFollowUserIDs(ctx, userID)
	if err != nil {
		return err
	}

	key := followKey(userID)
	pipe := s.rdb.TxPipeline()

	pipe.Del(ctx, key)

	if len(ids) > 0 {
		members := make([]any, 0, len(ids))
		for _, id := range ids {
			members = append(
				members,
				strconv.FormatInt(id, 10),
			)
		}

		pipe.SAdd(ctx, key, members...)
		pipe.Expire(ctx, key, followCacheTTL)
	}
	pipe.Set(
		ctx,
		followReadyKey(userID),
		"1",
		followReadyCacheTTL,
	)

	_, err = pipe.Exec(ctx)
	return err
}
