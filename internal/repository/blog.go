package repository

import (
	"context"
	"errors"
	"time"

	"github.com/learning/go-dianping/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) CreateBlog(
	ctx context.Context,
	blog *model.Blog,
) error {
	return r.DB.
		WithContext(ctx).
		Create(blog).
		Error
}

func (r *Repository) FindBlogByID(
	ctx context.Context,
	id int64,
) (model.Blog, error) {
	var blog model.Blog
	err := r.DB.
		WithContext(ctx).
		First(&blog, id).
		Error

	return blog, err
}

func (r *Repository) FindBlogsByUserID(
	ctx context.Context,
	userID int64,
	current int,
	pageSize int,
) ([]model.Blog, error) {
	var blogs []model.Blog

	offset := (current - 1) * pageSize

	err := r.DB.
		WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&blogs).
		Error

	return blogs, err
}

func (r *Repository) FindHotBlogs(
	ctx context.Context,
	current int,
	pageSize int,
) ([]model.Blog, error) {
	var blogs []model.Blog
	offset := (current - 1) * pageSize
	err := r.DB.
		WithContext(ctx).
		Order("liked DESC,id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&blogs).
		Error
	return blogs, err
}

func (r *Repository) ToggleBlogLike(
	ctx context.Context,
	blogID int64,
	userID int64,
) (bool, error) {
	liked := false
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var blog model.Blog
		//锁定笔记行，让同一笔记的点赞操作串行执行
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&blog, blogID).
			Error; err != nil {
			return err
		}
		var blogLike model.BlogLike
		err := tx.Where("blog_id = ? AND user_id= ? ", blogID, userID).
			First(&blogLike).
			Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			blogLike = model.BlogLike{
				BlogID: blogID,
				UserID: userID,
			}
			if err := tx.Create(&blogLike).Error; err != nil {
				return err
			}

			if err := tx.Model(&model.Blog{}).
				Where("id = ?", blogID).
				UpdateColumn("liked", gorm.Expr("liked + 1")).
				Error; err != nil {
				return err
			}

			liked = true
			return nil

		case err != nil:
			return err

		default:
			if err := tx.Delete(&blogLike).Error; err != nil {
				return err
			}

			if err := tx.Model(&model.Blog{}).
				Where("id = ?", blogID).
				UpdateColumn(
					"liked",
					gorm.Expr("GREATEST(liked - 1, 0)"),
				).
				Error; err != nil {
				return err
			}

			liked = false
			return nil
		}
	})
	if err != nil {
		return false, err
	}
	return liked, nil
}

func (r *Repository) FindEarliestBlogLikes(
	ctx context.Context,
	blogID int64,
	limit int,
) ([]model.BlogLike, error) {
	var likes []model.BlogLike

	err := r.DB.
		WithContext(ctx).
		Where("blog_id = ?", blogID).
		Order("create_time ASC, user_id ASC").
		Limit(limit).
		Find(&likes).
		Error

	return likes, err
}

func (r *Repository) FindBlogsByIDs(
	ctx context.Context,
	ids []int64,
) ([]model.Blog, error) {
	if len(ids) == 0 {
		return []model.Blog{}, nil
	}

	var blogs []model.Blog

	err := r.DB.
		WithContext(ctx).
		Where("id IN ?", ids).
		Find(&blogs).
		Error

	return blogs, err
}

func (r *Repository) CreateBlogWithFeed(
	ctx context.Context,
	blog *model.Blog,
) error {
	return r.DB.
		WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			// 1. 保存笔记。成功后 GORM 会回填 ID 和 CreateTime。
			if err := tx.Create(blog).Error; err != nil {
				return err
			}

			// 2. 在同一事务中查询发布者的粉丝。
			var followerIDs []int64
			if err := tx.
				Model(&model.Follow{}).
				Where("follow_user_id = ?", blog.UserID).
				Order("user_id ASC").
				Pluck("user_id", &followerIDs).
				Error; err != nil {
				return err
			}

			if len(followerIDs) == 0 {
				return nil
			}

			// 3. 为每个粉丝保存一条投递意图。
			outbox := make([]model.FeedOutbox, 0, len(followerIDs))
			score := blog.CreateTime.UnixMilli()

			for _, followerID := range followerIDs {
				outbox = append(outbox, model.FeedOutbox{
					UserID:     followerID,
					BlogID:     blog.ID,
					Score:      score,
					CreateTime: blog.CreateTime,
				})
			}

			return tx.CreateInBatches(outbox, 500).Error
		})
}

func (r *Repository) FindPendingFeedOutbox(
	ctx context.Context,
	limit int,
) ([]model.FeedOutbox, error) {
	if limit <= 0 {
		return []model.FeedOutbox{}, nil
	}

	var rows []model.FeedOutbox

	err := r.DB.
		WithContext(ctx).
		Where("delivered_at IS NULL").
		Order("id ASC").
		Limit(limit).
		Find(&rows).
		Error

	return rows, err
}

func (r *Repository) MarkFeedOutboxDelivered(
	ctx context.Context,
	ids []int64,
) error {
	if len(ids) == 0 {
		return nil
	}

	return r.DB.
		WithContext(ctx).
		Model(&model.FeedOutbox{}).
		Where("id IN ?", ids).
		Where("delivered_at IS NULL").
		Update("delivered_at", time.Now()).
		Error
}

func (r *Repository) FindFeedHistory(
	ctx context.Context,
	userID int64,
) ([]model.FeedOutbox, error) {
	var rows []model.FeedOutbox

	err := r.DB.
		WithContext(ctx).
		Where("user_id = ?", userID).
		Order("score DESC, blog_id DESC").
		Find(&rows).
		Error

	return rows, err
}
