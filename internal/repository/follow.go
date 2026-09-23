package repository

import (
	"context"

	"github.com/learning/go-dianping/internal/model"
	"gorm.io/gorm/clause"
)

func (r *Repository) SetFollow(
	ctx context.Context,
	userID int64,
	followUserID int64,
	isFollow bool,
) error {
	if isFollow {
		follow := model.Follow{
			UserID:       userID,
			FollowUserID: followUserID,
		}
		return r.DB.
			WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "user_id"},
					{Name: "follow_user_id"},
				},
				DoNothing: true,
			}).
			Create(&follow).
			Error
	}
	return r.DB.
		WithContext(ctx).
		Where(
			"user_id = ? AND follow_user_id = ?",
			userID,
			followUserID,
		).
		Delete(&model.Follow{}).
		Error
}

func (r *Repository) IsFollow(
	ctx context.Context,
	userID int64,
	followUserID int64,
) (bool, error) {
	var count int64
	err := r.DB.
		WithContext(ctx).
		Model(&model.Follow{}).
		Where(
			"user_id = ? AND follow_user_id = ?",
			userID,
			followUserID,
		).
		Count(&count).
		Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) FindCommonFollowUserIDs(
	ctx context.Context,
	userID int64,
	targetUserID int64,
) ([]int64, error) {
	var ids []int64
	targetFollows := r.DB.
		WithContext(ctx).
		Model(&model.Follow{}).
		Select("follow_user_id").
		Where("user_id = ?", targetUserID)

	err := r.DB.
		WithContext(ctx).
		Model(&model.Follow{}).
		Where("user_id = ?", userID).
		Where("follow_user_id IN (?)", targetFollows).
		Order("follow_user_id ASC").
		Pluck("follow_user_id", &ids).
		Error
	return ids, err
}

func (r *Repository) FindFollowUserIDs(
	ctx context.Context,
	userID int64,
) ([]int64, error) {
	var ids []int64
	err := r.DB.
		WithContext(ctx).
		Model(&model.Follow{}).
		Where("user_id = ?", userID).
		Order("follow_user_id ASC").
		Pluck("follow_user_id", &ids).
		Error
	return ids, err
}

func (r *Repository) FindFollowerUserIDs(
	ctx context.Context,
	followUserID int64,
) ([]int64, error) {
	var ids []int64

	err := r.DB.
		WithContext(ctx).
		Model(&model.Follow{}).
		Where("follow_user_id = ?", followUserID).
		Order("user_id ASC").
		Pluck("user_id", &ids).
		Error

	return ids, err
}
