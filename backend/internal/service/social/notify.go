package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Notification 是站内通知的服务层视图（需求 §12.3）。
type Notification struct {
	ID        int64
	Kind      domain.NotificationKind
	Title     string
	Body      string
	Link      string
	Read      bool
	CreatedAt time.Time
}

// NotificationPage 是通知列表的一页。
type NotificationPage struct {
	Items    []Notification
	Page     int32
	PageSize int32
	Total    int64
}

// notification 是一条待写入的通知。
type notification struct {
	UserID  int64
	Kind    domain.NotificationKind
	Title   string
	Body    string
	Link    string
	Payload map[string]any
}

// notify 写入一条站内通知。通知与触发它的业务变更同事务：
// 出现「评论已回复但通知未发出」的状态会让收件人永久看不到这条回复。
func (s *Service) notify(ctx context.Context, q *repo.Queries, n notification) error {
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		return fmt.Errorf("序列化通知载荷: %w", err)
	}
	if n.Payload == nil {
		payload = []byte("{}")
	}
	if _, err := q.InsertNotification(ctx, repo.InsertNotificationParams{
		UserID:  n.UserID,
		Kind:    string(n.Kind),
		Title:   n.Title,
		Body:    n.Body,
		Link:    n.Link,
		Payload: payload,
	}); err != nil {
		return fmt.Errorf("写入通知（用户 %d）: %w", n.UserID, err)
	}
	return nil
}

// NotifyLibraryQuestionDeleted 发出协作者删题通知（需求 §10.6、C-34）。
//
// 供题库域在删除题目后调用：通知与删除必须同事务，否则会出现删了题却没通知的状态。
// 正文文案取自 domain，使通知内容只有一个来源，题库域的测试与实现依据同一份定义。
func (s *Service) NotifyLibraryQuestionDeleted(
	ctx context.Context, q *repo.Queries, ownerID int64, libraryName, questionTitle, actorName string,
) error {
	return s.notify(ctx, q, notification{
		UserID: ownerID,
		Kind:   domain.NotifyLibraryQuestionDeleted,
		Title:  "题库题目被协作者删除",
		Body:   domain.NotificationLibraryQuestionDeletedBody(actorName, libraryName, questionTitle),
		Payload: map[string]any{
			"library_name":   libraryName,
			"question_title": questionTitle,
			"actor_name":     actorName,
		},
	})
}

// ListNotifications 返回当前用户的通知，可按类型筛选（需求 §12.3）。
//
// kinds 为空表示不筛选。
func (s *Service) ListNotifications(
	ctx context.Context, v domain.Viewer, kinds []domain.NotificationKind, page, pageSize int32,
) (NotificationPage, error) {
	if err := s.ensure(); err != nil {
		return NotificationPage{}, translatePermission(err)
	}
	if !v.Logged {
		return NotificationPage{}, apperr.New(apperr.CodeUnauthorized, "")
	}

	raw := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		raw = append(raw, string(kind))
	}

	q := repo.New(s.pool)
	rows, err := q.ListNotifications(ctx, repo.ListNotificationsParams{
		UserID:      v.UserID,
		Kinds:       raw,
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return NotificationPage{}, wrap("查询通知列表", v.UserID, err)
	}
	total, err := q.CountNotifications(ctx, repo.CountNotificationsParams{
		UserID: v.UserID,
		Kinds:  raw,
	})
	if err != nil {
		return NotificationPage{}, wrap("统计通知数量", v.UserID, err)
	}

	items := make([]Notification, 0, len(rows))
	for _, row := range rows {
		groups, ok := domain.ParseNotificationKind(row.Kind)
		if !ok {
			return NotificationPage{}, fmt.Errorf("通知 %d 的类型取值 %q 未登记", row.ID, row.Kind)
		}
		item := Notification{
			ID:    row.ID,
			Kind:  groups,
			Title: row.Title,
			Body:  row.Body,
			Link:  row.Link,
			Read:  row.ReadAt.Valid,
		}
		if row.CreatedAt.Valid {
			item.CreatedAt = row.CreatedAt.Time
		}
		items = append(items, item)
	}
	return NotificationPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// UnreadCount 返回未读通知数（需求 §12.3「查看未读数量」）。
//
// 未读数包含两部分：站内通知与未读公告。公告不扇出为通知（C-33），
// 因此未读公告由水位线单独统计，两个来源相加才是用户在通知入口看到的数字。
func (s *Service) UnreadCount(ctx context.Context, v domain.Viewer) (int64, error) {
	if err := s.ensure(); err != nil {
		return 0, translatePermission(err)
	}
	if !v.Logged {
		return 0, apperr.New(apperr.CodeUnauthorized, "")
	}

	q := repo.New(s.pool)
	notifications, err := q.CountUnreadNotifications(ctx, v.UserID)
	if err != nil {
		return 0, wrap("统计未读通知", v.UserID, err)
	}
	announcements, err := q.CountUnreadAnnouncements(ctx, v.UserID)
	if err != nil {
		return 0, wrap("统计未读公告", v.UserID, err)
	}
	return notifications + announcements, nil
}

// MarkNotificationRead 标记单条通知已读。
func (s *Service) MarkNotificationRead(ctx context.Context, v domain.Viewer, id int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.Logged {
		return apperr.New(apperr.CodeUnauthorized, "")
	}
	if _, err := repo.New(s.pool).MarkNotificationRead(ctx, repo.MarkNotificationReadParams{
		ID:     id,
		UserID: v.UserID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound("通知")
		}
		return wrap("标记通知已读", id, err)
	}
	return nil
}

// MarkAllNotificationsRead 全部标记已读（需求 §12.3）。
func (s *Service) MarkAllNotificationsRead(ctx context.Context, v domain.Viewer) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.Logged {
		return apperr.New(apperr.CodeUnauthorized, "")
	}
	if err := repo.New(s.pool).MarkAllNotificationsRead(ctx, v.UserID); err != nil {
		return wrap("全部标记已读", v.UserID, err)
	}
	return nil
}

// DeleteNotification 删除通知（需求 §12.3）。
func (s *Service) DeleteNotification(ctx context.Context, v domain.Viewer, id int64) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.Logged {
		return apperr.New(apperr.CodeUnauthorized, "")
	}
	if _, err := repo.New(s.pool).SoftDeleteNotification(ctx, repo.SoftDeleteNotificationParams{
		ID:     id,
		UserID: v.UserID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound("通知")
		}
		return wrap("删除通知", id, err)
	}
	return nil
}

// Announcement 是一条公告（需求 §12.3）。
type Announcement struct {
	ID          int64
	Title       string
	Body        string
	PublishedAt time.Time
	Read        bool
}

// AnnouncementPage 是公告列表的一页。
type AnnouncementPage struct {
	Items    []Announcement
	Page     int32
	PageSize int32
	Total    int64
}

// ListAnnouncements 返回未撤回的公告（C-33）。
//
// 已读状态由发布时刻与水位线的比较得出，不逐条存已读标记：
// 公告只存一份，撤回后立即从本查询结果中消失，不留残余通知。
func (s *Service) ListAnnouncements(ctx context.Context, v domain.Viewer, page, pageSize int32) (AnnouncementPage, error) {
	if err := s.ensure(); err != nil {
		return AnnouncementPage{}, translatePermission(err)
	}

	q := repo.New(s.pool)
	rows, err := q.ListLiveAnnouncements(ctx, repo.ListLiveAnnouncementsParams{
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return AnnouncementPage{}, wrap("查询公告列表", 0, err)
	}
	total, err := q.CountLiveAnnouncements(ctx)
	if err != nil {
		return AnnouncementPage{}, wrap("统计公告数量", 0, err)
	}

	watermark, err := s.readWatermark(ctx, q, v)
	if err != nil {
		return AnnouncementPage{}, err
	}

	items := make([]Announcement, 0, len(rows))
	for _, row := range rows {
		item := Announcement{ID: row.ID, Title: row.Title, Body: row.Body}
		if row.PublishedAt.Valid {
			item.PublishedAt = row.PublishedAt.Time
			// 水位线为空表示从未读过任何公告，此时全部视为未读。
			item.Read = watermark != nil && !row.PublishedAt.Time.After(*watermark)
		}
		items = append(items, item)
	}
	return AnnouncementPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// MarkAnnouncementsRead 把水位线推到当前时刻：所有已发布的公告从此视为已读（C-33）。
func (s *Service) MarkAnnouncementsRead(ctx context.Context, v domain.Viewer) error {
	if err := s.ensure(); err != nil {
		return translatePermission(err)
	}
	if !v.Logged {
		return apperr.New(apperr.CodeUnauthorized, "")
	}
	// 水位线取当前时刻而非最后一条公告的发布时间：撤回后再发布的公告
	// 若沿用旧水位线会被判为新公告，与「已读全部」的语义不符。
	now := s.now().UTC()
	if err := repo.New(s.pool).AdvanceAnnouncementWatermark(ctx, repo.AdvanceAnnouncementWatermarkParams{
		UserID: v.UserID,
		ReadAt: repo.Timestamptz(now),
	}); err != nil {
		return wrap("推进公告水位线", v.UserID, err)
	}
	return nil
}

// readWatermark 读取当前用户的公告已读水位线。从未读过时为 nil。
func (s *Service) readWatermark(ctx context.Context, q *repo.Queries, v domain.Viewer) (*time.Time, error) {
	if !v.Logged {
		return nil, nil
	}
	row, err := q.GetAnnouncementWatermark(ctx, v.UserID)
	if err != nil {
		return nil, wrap("读取公告水位线", v.UserID, err)
	}
	if !row.Valid {
		return nil, nil
	}
	value := row.Time
	return &value, nil
}

// promptLink 返回提示词详情页的站内链接。
func promptLink(promptID int64) string {
	return fmt.Sprintf("/prompts/%d", promptID)
}
