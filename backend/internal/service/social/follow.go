package social

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Profile 是被关注用户的公开资料。
type Profile struct {
	ID         int64
	Username   string
	Bio        string
	AvatarURL  string
	Followed   bool
	FollowedAt time.Time
}

// ProfilePage 是关注列表的一页。
type ProfilePage struct {
	Items    []Profile
	Page     int32
	PageSize int32
	Total    int64
}

// Follow 关注一个用户（需求 §9.4）。
//
// 关注关系是社交链路而不是身份授权：它只让人方便地查看被关注者的公开收藏夹，
// 不授予任何访问私有内容的权限。
func (s *Service) Follow(ctx context.Context, v domain.Viewer, followeeID int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := domain.ValidateFollow(v.UserID, followeeID); err != nil {
		return apperr.New(apperr.CodeValidationFailed, err.Error())
	}

	q := repo.New(s.pool)
	if _, err := q.GetFolloweeProfile(ctx, followeeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound("用户")
		}
		return wrap("查询被关注用户", followeeID, err)
	}

	// 已关注时 ON CONFLICT DO NOTHING 不返回行，视为幂等成功：
	// 重复点击关注按钮不应该报错。
	if _, err := q.InsertFollow(ctx, repo.InsertFollowParams{
		FollowerID: v.UserID,
		FolloweeID: followeeID,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return wrap("写入关注关系", followeeID, err)
	}
	return nil
}

// Unfollow 取消关注。
func (s *Service) Unfollow(ctx context.Context, v domain.Viewer, followeeID int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := repo.New(s.pool).DeleteFollow(ctx, repo.DeleteFollowParams{
		FollowerID: v.UserID,
		FolloweeID: followeeID,
	}); err != nil {
		return wrap("删除关注关系", followeeID, err)
	}
	return nil
}

// ListFollowing 返回当前用户关注的人。
func (s *Service) ListFollowing(ctx context.Context, v domain.Viewer, page, pageSize int32) (ProfilePage, error) {
	if err := s.ensure(); err != nil {
		return ProfilePage{}, translatePermission(err)
	}
	if !v.Logged {
		return ProfilePage{}, apperr.New(apperr.CodeUnauthorized, "")
	}

	q := repo.New(s.pool)
	rows, err := q.ListFollowing(ctx, repo.ListFollowingParams{
		FollowerID:  v.UserID,
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return ProfilePage{}, wrap("查询关注列表", v.UserID, err)
	}
	total, err := q.CountFollowing(ctx, v.UserID)
	if err != nil {
		return ProfilePage{}, wrap("统计关注数量", v.UserID, err)
	}

	items := make([]Profile, 0, len(rows))
	for _, row := range rows {
		item := Profile{
			ID:        row.ID,
			Username:  row.Username,
			Bio:       row.Bio,
			AvatarURL: row.AvatarUrl,
			Followed:  true,
		}
		if row.FollowedAt.Valid {
			item.FollowedAt = row.FollowedAt.Time
		}
		items = append(items, item)
	}
	return ProfilePage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// UserProfile 返回某个用户的公开资料与关注状态，供查看他人收藏夹时使用。
func (s *Service) UserProfile(ctx context.Context, v domain.Viewer, userID int64) (Profile, error) {
	if err := s.ensure(); err != nil {
		return Profile{}, translatePermission(err)
	}

	q := repo.New(s.pool)
	row, err := q.GetFolloweeProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Profile{}, notFound("用户")
		}
		return Profile{}, wrap("查询用户", userID, err)
	}

	out := Profile{ID: row.ID, Username: row.Username, Bio: row.Bio, AvatarURL: row.AvatarUrl}
	if v.Logged {
		followed, err := q.IsFollowing(ctx, repo.IsFollowingParams{
			FollowerID: v.UserID,
			FolloweeID: userID,
		})
		if err != nil {
			return Profile{}, wrap("查询关注状态", userID, err)
		}
		out.Followed = followed
	}
	return out, nil
}
