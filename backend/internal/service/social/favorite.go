package social

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// Favorite 是收藏夹的服务层视图。
//
// 可见性判定委托给 domain.Favorite：服务层不重复实现判定规则，
// 只附加展示所需的字段（所有者昵称、创建与更新时间）。
type Favorite struct {
	Entity      domain.Favorite
	OwnerName   string
	OwnerAvatar string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Access 返回可见性判定要素，转发到领域实体。
func (f Favorite) Access() domain.Access { return f.Entity.Access() }

// FavoriteItem 是收藏夹里的一条提示词。
type FavoriteItem struct {
	Prompt         domain.Prompt
	CategoryNameZh string
	CategoryNameEn string
	AddedAt        time.Time
}

// FavoritePage 是收藏夹列表的一页。
type FavoritePage struct {
	Items    []Favorite
	Page     int32
	PageSize int32
	Total    int64
}

// FavoriteItemPage 是收藏夹条目列表的一页。
type FavoriteItemPage struct {
	Items    []FavoriteItem
	Page     int32
	PageSize int32
	Total    int64
}

// FavoriteInput 是创建与修改收藏夹的输入。
type FavoriteInput struct {
	Name        string
	Description string
	Visibility  domain.FavoriteVisibility
}

// CreateFavorite 创建收藏夹。
//
// 配额判定在事务内完成（C-21）：先取该用户的咨询锁，再计数、判定、写入。
// 事务外计数后插入会让并发请求各自读到未超限的计数，双双写入成功而突破上限。
func (s *Service) CreateFavorite(ctx context.Context, v domain.Viewer, in FavoriteInput) (Favorite, error) {
	if err := s.ensure(); err != nil {
		return Favorite{}, translatePermission(err)
	}
	if !v.CanWrite() {
		return Favorite{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := validateFavoriteInput(in); err != nil {
		return Favorite{}, err
	}

	// 配额按身份取值，可由后台调整（可行性报告 §2.14 的两个键）。
	key := "favorite_count_limit_user"
	fallback := int64(domain.FavoriteCountLimitUser)
	if v.IsDeveloper() {
		key = "favorite_count_limit_developer"
		fallback = int64(domain.FavoriteCountLimitDeveloper)
	}
	limit := s.intOf(ctx, key, fallback)

	var created Favorite
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if err := q.LockFavoriteOwner(ctx, v.UserID); err != nil {
			return wrap("获取收藏夹配额锁", v.UserID, err)
		}
		count, err := q.CountFavorites(ctx, v.UserID)
		if err != nil {
			return wrap("统计收藏夹数量", v.UserID, err)
		}
		if count >= limit {
			return apperr.Errorf(apperr.CodeQuotaExceeded, "收藏夹数量已达上限 %d 个", limit)
		}

		row, err := q.InsertFavorite(ctx, repo.InsertFavoriteParams{
			UserID:      v.UserID,
			Name:        in.Name,
			Description: in.Description,
			Visibility:  string(in.Visibility),
		})
		if err != nil {
			return wrap("写入收藏夹", v.UserID, err)
		}
		favorite, err := favoriteFromRow(row, "")
		if err != nil {
			return err
		}
		created = favorite
		return nil
	})
	if err != nil {
		return Favorite{}, err
	}
	return created, nil
}

// UpdateFavorite 修改收藏夹名称、描述与可见性。仅所有者与管理员可改。
func (s *Service) UpdateFavorite(ctx context.Context, v domain.Viewer, id int64, in FavoriteInput) (Favorite, error) {
	if err := s.ensure(); err != nil {
		return Favorite{}, translatePermission(err)
	}
	if !v.CanWrite() {
		return Favorite{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := validateFavoriteInput(in); err != nil {
		return Favorite{}, err
	}

	var updated Favorite
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if _, err := loadManageableFavorite(ctx, q, v, id); err != nil {
			return err
		}
		row, err := q.UpdateFavorite(ctx, repo.UpdateFavoriteParams{
			ID:          id,
			Name:        in.Name,
			Description: in.Description,
			Visibility:  string(in.Visibility),
		})
		if err != nil {
			return wrap("更新收藏夹", id, err)
		}
		result, err := favoriteFromRow(row, "")
		if err != nil {
			return err
		}
		updated = result
		return nil
	})
	if err != nil {
		return Favorite{}, err
	}
	return updated, nil
}

// DeleteFavorite 删除收藏夹。
//
// 初版为直接删除而非进入回收站（需求 §11.2），但必须记录日志。
// 日志与删除同事务：出现「已删除但无记录」的状态会让审计出现缺口。
func (s *Service) DeleteFavorite(ctx context.Context, v domain.Viewer, id int64, ip string) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		favorite, err := loadManageableFavorite(ctx, q, v, id)
		if err != nil {
			return err
		}
		if err := q.DeleteFavoriteItems(ctx, id); err != nil {
			return wrap("清空收藏夹条目", id, err)
		}
		if _, err := q.SoftDeleteFavorite(ctx, id); err != nil {
			return wrap("删除收藏夹", id, err)
		}
		return s.record(ctx, q, oplogEntry{
			Action:     oplog.ActionFavoriteDelete,
			Actor:      v,
			ObjectType: "favorite",
			ObjectID:   id,
			Before: map[string]any{
				"name":       favorite.Entity.Name,
				"visibility": string(favorite.Entity.Visibility),
				"item_count": favorite.Entity.ItemCount,
			},
			IP: ip,
		})
	})
}

// ListFavorites 返回某个用户的收藏夹。
//
// 本人与管理员看到全部，其余身份只看到公开收藏夹（需求 §9.3）。
// 判定复用 domain.Viewer，不在此处另写一套。
func (s *Service) ListFavorites(ctx context.Context, v domain.Viewer, ownerID int64, page, pageSize int32) (FavoritePage, error) {
	if err := s.ensure(); err != nil {
		return FavoritePage{}, translatePermission(err)
	}

	q := repo.New(s.pool)
	owner, err := q.GetFolloweeProfile(ctx, ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FavoritePage{}, notFound("用户")
		}
		return FavoritePage{}, wrap("查询用户", ownerID, err)
	}

	includePrivate := (v.Logged && v.UserID == ownerID) || v.IsAdmin()

	rows, err := q.ListFavorites(ctx, repo.ListFavoritesParams{
		OwnerID:        ownerID,
		IncludePrivate: includePrivate,
		LimitCount:     pageSize,
		OffsetCount:    (page - 1) * pageSize,
	})
	if err != nil {
		return FavoritePage{}, wrap("查询收藏夹列表", ownerID, err)
	}
	total, err := q.CountVisibleFavorites(ctx, repo.CountVisibleFavoritesParams{
		OwnerID:        ownerID,
		IncludePrivate: includePrivate,
	})
	if err != nil {
		return FavoritePage{}, wrap("统计收藏夹数量", ownerID, err)
	}

	items := make([]Favorite, 0, len(rows))
	for _, row := range rows {
		item, err := favoriteFromRow(repo.Favorite{
			ID: row.ID, UserID: row.UserID, Name: row.Name, Description: row.Description,
			Visibility: row.Visibility, ItemCount: row.ItemCount,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}, owner.Username)
		if err != nil {
			return FavoritePage{}, err
		}
		item.OwnerAvatar = owner.AvatarUrl
		items = append(items, item)
	}
	return FavoritePage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// AddItem 把提示词加入收藏夹。
//
// 条目上限与插入在同一事务内判定，条件 UPDATE 保证并发下不会越过上限：
// 即便两个请求同时读到 item_count 未超限，也只有一个能命中 increment 的 WHERE。
func (s *Service) AddItem(ctx context.Context, v domain.Viewer, favoriteID, promptID int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		favorite, err := loadManageableFavorite(ctx, q, v, favoriteID)
		if err != nil {
			return err
		}
		if _, err := loadVisiblePrompt(ctx, q, v, promptID); err != nil {
			return err
		}

		if _, err := q.InsertFavoriteItem(ctx, repo.InsertFavoriteItemParams{
			FavoriteID: favorite.Entity.ID,
			PromptID:   promptID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidState, "该提示词已在此收藏夹中")
			}
			return wrap("写入收藏夹条目", favoriteID, err)
		}

		// 条件 UPDATE 未命中说明计数已达上限，返回错误让整个事务回滚，
		// 上面插入的条目随之撤销。
		limit := int32(s.intOf(ctx, "favorite_item_limit", domain.FavoriteItemLimit))
		if _, err := q.IncrementFavoriteItemCount(ctx, repo.IncrementFavoriteItemCountParams{
			ID:         favorite.Entity.ID,
			LimitCount: limit,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Errorf(apperr.CodeQuotaExceeded, "收藏夹最多容纳 %d 条提示词", limit)
			}
			return wrap("更新收藏夹条目数", favoriteID, err)
		}
		return nil
	})
}

// RemoveItem 从收藏夹移除提示词。
func (s *Service) RemoveItem(ctx context.Context, v domain.Viewer, favoriteID, promptID int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		favorite, err := loadManageableFavorite(ctx, q, v, favoriteID)
		if err != nil {
			return err
		}
		if _, err := q.DeleteFavoriteItem(ctx, repo.DeleteFavoriteItemParams{
			FavoriteID: favorite.Entity.ID,
			PromptID:   promptID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("收藏夹条目")
			}
			return wrap("删除收藏夹条目", favoriteID, err)
		}
		if err := q.DecrementFavoriteItemCount(ctx, favorite.Entity.ID); err != nil {
			return wrap("更新收藏夹条目数", favoriteID, err)
		}
		return nil
	})
}

// ListItems 返回收藏夹里的条目。
//
// 逐条复查可见性（开发方案 §6.5 验收标准）：夹内的提示词可能在上次访问后被改为
// 私有或高风险，此时不能再展示。判定复用 domain.Viewer.CanView，与加入条目时的
// 判定同源。
func (s *Service) ListItems(ctx context.Context, v domain.Viewer, favoriteID int64, page, pageSize int32) (FavoriteItemPage, error) {
	if err := s.ensure(); err != nil {
		return FavoriteItemPage{}, translatePermission(err)
	}

	q := repo.New(s.pool)
	favorite, err := loadViewableFavorite(ctx, q, v, favoriteID)
	if err != nil {
		return FavoriteItemPage{}, err
	}

	visible, err := s.visibleItems(ctx, q, v, favorite.Entity.ID)
	if err != nil {
		return FavoriteItemPage{}, err
	}

	// 先逐条复查可见性再分页：不可见的条目要从结果里剔除而不是占一个位置，
	// 否则分页会显示多余的空页，且总数与列表口径不一致。
	offset := int((page - 1) * pageSize)
	limit := int(pageSize)
	if offset > len(visible) {
		offset = len(visible)
	}
	end := offset + limit
	if end > len(visible) {
		end = len(visible)
	}
	items := visible[offset:end]
	return FavoriteItemPage{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    int64(len(visible)),
	}, nil
}

// visibleItems 返回收藏夹内对当前身份可见的全部条目。
//
// 可见性谓词只存在于 domain.Viewer，因此这里逐页取回后过滤，
// 而不是在 SQL 里另写一份谓词（开发方案 §3.1）。
func (s *Service) visibleItems(ctx context.Context, q *repo.Queries, v domain.Viewer, favoriteID int64) ([]FavoriteItem, error) {
	const batch = int32(200)
	out := []FavoriteItem{}
	for offset := int32(0); ; offset += batch {
		rows, err := q.ListFavoriteItems(ctx, repo.ListFavoriteItemsParams{
			FavoriteID:  favoriteID,
			LimitCount:  batch,
			OffsetCount: offset,
		})
		if err != nil {
			return nil, wrap("查询收藏夹条目", favoriteID, err)
		}
		for _, row := range rows {
			prompt, err := promptFromItemRow(row)
			if err != nil {
				return nil, err
			}
			if !v.CanView(prompt.Access()) {
				continue
			}
			item := FavoriteItem{
				Prompt:         prompt,
				CategoryNameZh: row.CategoryNameZh,
				CategoryNameEn: row.CategoryNameEn,
			}
			if row.AddedAt.Valid {
				item.AddedAt = row.AddedAt.Time
			}
			out = append(out, item)
		}
		if int32(len(rows)) < batch {
			return out, nil
		}
	}
}

// IsFavorited 判定某提示词是否已被当前用户收藏到任意收藏夹。
func (s *Service) IsFavorited(ctx context.Context, v domain.Viewer, promptID int64) (bool, error) {
	if err := s.ensure(); err != nil {
		return false, translatePermission(err)
	}
	if !v.Logged {
		return false, nil
	}
	favorited, err := repo.New(s.pool).IsPromptFavorited(ctx, repo.IsPromptFavoritedParams{
		UserID:   v.UserID,
		PromptID: promptID,
	})
	if err != nil {
		return false, wrap("判定收藏状态", promptID, err)
	}
	return favorited, nil
}

// validateFavoriteInput 校验收藏夹的输入。
func validateFavoriteInput(in FavoriteInput) error {
	if len([]rune(in.Name)) == 0 {
		return apperr.New(apperr.CodeValidationFailed, "收藏夹名称不能为空")
	}
	if len([]rune(in.Name)) > favoriteNameMaxRunes {
		return apperr.Errorf(apperr.CodeValidationFailed,
			"收藏夹名称不能超过 %d 个字", favoriteNameMaxRunes)
	}
	if len([]rune(in.Description)) > favoriteDescriptionMaxRunes {
		return apperr.Errorf(apperr.CodeValidationFailed,
			"收藏夹描述不能超过 %d 个字", favoriteDescriptionMaxRunes)
	}
	return nil
}

const (
	// favoriteNameMaxRunes 是收藏夹名称的字数上限。
	favoriteNameMaxRunes = 100
	// favoriteDescriptionMaxRunes 是收藏夹描述的字数上限。
	favoriteDescriptionMaxRunes = 500
)

// favoriteFromRow 把数据库行映射为服务层结构。
func favoriteFromRow(row repo.Favorite, ownerName string) (Favorite, error) {
	visibility, ok := domain.ParseFavoriteVisibility(row.Visibility)
	if !ok {
		return Favorite{}, fmt.Errorf("收藏夹 %d 的可见性取值 %q 未登记", row.ID, row.Visibility)
	}
	out := Favorite{
		Entity: domain.Favorite{
			ID:          row.ID,
			UserID:      row.UserID,
			Name:        row.Name,
			Description: row.Description,
			Visibility:  visibility,
			ItemCount:   row.ItemCount,
		},
		OwnerName: ownerName,
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		out.UpdatedAt = row.UpdatedAt.Time
	}
	return out, nil
}

// loadViewableFavorite 读取当前身份可见的收藏夹。
func loadViewableFavorite(ctx context.Context, q *repo.Queries, v domain.Viewer, id int64) (Favorite, error) {
	row, err := q.GetFavorite(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Favorite{}, notFound("收藏夹")
		}
		return Favorite{}, wrap("查询收藏夹", id, err)
	}
	favorite, err := favoriteFromRow(row, "")
	if err != nil {
		return Favorite{}, err
	}
	if !v.CanView(favorite.Access()) {
		return Favorite{}, notFound("收藏夹")
	}
	return favorite, nil
}

// loadManageableFavorite 读取当前身份可管理的收藏夹：所有者或管理员。
//
// 添加与移除条目要求管理权限而不是可见权限：公开收藏夹对所有人可见，
// 但只有所有者能往里加东西。
func loadManageableFavorite(ctx context.Context, q *repo.Queries, v domain.Viewer, id int64) (Favorite, error) {
	row, err := q.GetFavoriteForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Favorite{}, notFound("收藏夹")
		}
		return Favorite{}, wrap("查询收藏夹", id, err)
	}
	favorite, err := favoriteFromRow(row, "")
	if err != nil {
		return Favorite{}, err
	}
	if !v.CanManage(favorite.Access()) {
		return Favorite{}, notFound("收藏夹")
	}
	return favorite, nil
}

// loadVisiblePrompt 读取当前身份可见的提示词。
func loadVisiblePrompt(ctx context.Context, q *repo.Queries, v domain.Viewer, id int64) (domain.Prompt, error) {
	row, err := q.GetPrompt(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Prompt{}, notFound("提示词")
		}
		return domain.Prompt{}, wrap("查询提示词", id, err)
	}
	prompt, err := promptFromRow(row)
	if err != nil {
		return domain.Prompt{}, err
	}
	if !v.CanView(prompt.Access()) {
		return domain.Prompt{}, notFound("提示词")
	}
	return prompt, nil
}
