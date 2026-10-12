package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/social"
)

func (s *Server) registerSocialRoutes() {
	// ---------- 收藏夹 ----------

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/users/{id}/favorites",
		OperationID: "listUserFavorites", Summary: "用户的收藏夹", Tags: []string{"收藏夹"},
	}, s.handleListUserFavorites)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/favorites/{id}/items",
		OperationID: "listFavoriteItems", Summary: "收藏夹条目", Tags: []string{"收藏夹"},
	}, s.handleListFavoriteItems)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/favorites",
		OperationID: "listMyFavorites", Summary: "我的收藏夹", Tags: []string{"收藏夹"},
	}, s.handleListMyFavorites)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/favorited/{prompt_id}",
		OperationID: "isPromptFavorited", Summary: "该提示词是否已被我收藏", Tags: []string{"收藏夹"},
	}, s.handleIsFavorited)

	Register(s, "favorite_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/favorites",
		OperationID: "createFavorite", Summary: "创建收藏夹", Tags: []string{"收藏夹"},
	}, s.handleCreateFavorite)

	Register(s, "favorite_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/favorites/{id}",
		OperationID: "updateFavorite", Summary: "修改收藏夹", Tags: []string{"收藏夹"},
	}, s.handleUpdateFavorite)

	Register(s, "favorite_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/favorites/{id}",
		OperationID: "deleteFavorite", Summary: "删除收藏夹", Tags: []string{"收藏夹"},
	}, s.handleDeleteFavorite)

	Register(s, "favorite_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/favorites/{id}/items/{prompt_id}",
		OperationID: "addFavoriteItem", Summary: "加入收藏夹", Tags: []string{"收藏夹"},
	}, s.handleAddFavoriteItem)

	Register(s, "favorite_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/favorites/{id}/items/{prompt_id}",
		OperationID: "removeFavoriteItem", Summary: "移出收藏夹", Tags: []string{"收藏夹"},
	}, s.handleRemoveFavoriteItem)

	// ---------- 关注 ----------

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/users/{id}/profile",
		OperationID: "getUserProfile", Summary: "用户公开资料", Tags: []string{"关注"},
	}, s.handleGetUserProfile)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/following",
		OperationID: "listFollowing", Summary: "我的关注列表", Tags: []string{"关注"},
	}, s.handleListFollowing)

	Register(s, "favorite_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/me/following/{id}",
		OperationID: "followUser", Summary: "关注用户", Tags: []string{"关注"},
	}, s.handleFollowUser)

	Register(s, "favorite_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/me/following/{id}",
		OperationID: "unfollowUser", Summary: "取消关注", Tags: []string{"关注"},
	}, s.handleUnfollowUser)

	// ---------- 评论 ----------

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/prompts/{id}/comments",
		OperationID: "listComments", Summary: "评论列表", Tags: []string{"评论"},
	}, s.handleListComments)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/comments/{id}/replies",
		OperationID: "listReplies", Summary: "评论的回复", Tags: []string{"评论"},
	}, s.handleListReplies)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/prompts/{id}/comments",
		OperationID: "createComment", Summary: "发表评论或回复", Tags: []string{"评论"},
	}, s.handleCreateComment)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/comments/{id}",
		OperationID: "deleteComment", Summary: "删除自己的评论", Tags: []string{"评论"},
	}, s.handleDeleteComment)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/comments/{id}/like",
		OperationID: "likeComment", Summary: "点赞评论", Tags: []string{"评论"},
	}, s.handleLikeComment)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/comments/{id}/like",
		OperationID: "unlikeComment", Summary: "取消点赞", Tags: []string{"评论"},
	}, s.handleUnlikeComment)

	// ---------- 举报 ----------

	Register(s, "report_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/reports",
		OperationID: "createReport", Summary: "提交举报", Tags: []string{"举报"},
	}, s.handleCreateReport)

	// ---------- 通知与公告 ----------

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/announcements",
		OperationID: "listAnnouncements", Summary: "公告列表", Tags: []string{"通知"},
	}, s.handleListAnnouncements)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/notifications",
		OperationID: "listNotifications", Summary: "通知列表", Tags: []string{"通知"},
	}, s.handleListNotifications)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/notifications/unread-count",
		OperationID: "unreadNotificationCount", Summary: "未读数量", Tags: []string{"通知"},
	}, s.handleUnreadCount)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/me/notifications/read-all",
		OperationID: "markAllNotificationsRead", Summary: "全部标记已读", Tags: []string{"通知"},
	}, s.handleMarkAllNotificationsRead)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/me/announcements/read-all",
		OperationID: "markAnnouncementsRead", Summary: "公告全部标记已读", Tags: []string{"通知"},
	}, s.handleMarkAnnouncementsRead)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/me/notifications/{id}/read",
		OperationID: "markNotificationRead", Summary: "标记通知已读", Tags: []string{"通知"},
	}, s.handleMarkNotificationRead)

	Register(s, "comment_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/me/notifications/{id}",
		OperationID: "deleteNotification", Summary: "删除通知", Tags: []string{"通知"},
	}, s.handleDeleteNotification)
}

// ---------- 收藏夹 ----------

type favoriteBody struct {
	Name        string `json:"name" required:"true" minLength:"1" maxLength:"100" doc:"收藏夹名称"`
	Description string `json:"description,omitempty" maxLength:"500" doc:"描述说明"`
	Visibility  string `json:"visibility,omitempty" default:"private" enum:"public,private" doc:"可见性，默认私有"`
}

func (b favoriteBody) toInput() (social.FavoriteInput, error) {
	visibility := b.Visibility
	if visibility == "" {
		visibility = string(domain.FavoritePrivate)
	}
	parsed, ok := domain.ParseFavoriteVisibility(visibility)
	if !ok {
		return social.FavoriteInput{}, apperr.New(apperr.CodeValidationFailed, "可见性取值非法")
	}
	return social.FavoriteInput{
		Name:        b.Name,
		Description: b.Description,
		Visibility:  parsed,
	}, nil
}

type favoriteDTO struct {
	ID          int64  `json:"id"`
	OwnerID     int64  `json:"owner_id"`
	OwnerName   string `json:"owner_name,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility" enum:"public,private"`
	ItemCount   int32  `json:"item_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toFavoriteDTO(f social.Favorite) favoriteDTO {
	return favoriteDTO{
		ID:          f.Entity.ID,
		OwnerID:     f.Entity.UserID,
		OwnerName:   f.OwnerName,
		Name:        f.Entity.Name,
		Description: f.Entity.Description,
		Visibility:  string(f.Entity.Visibility),
		ItemCount:   f.Entity.ItemCount,
		CreatedAt:   formatTime(f.CreatedAt),
		UpdatedAt:   formatTime(f.UpdatedAt),
	}
}

type favoriteListResponse struct {
	Body struct {
		Items    []favoriteDTO `json:"items"`
		Page     int32         `json:"page"`
		PageSize int32         `json:"page_size"`
		Total    int64         `json:"total"`
	}
}

type favoriteWriteResponse struct {
	Body struct {
		Favorite favoriteDTO `json:"favorite"`
	}
}

type favoriteItemDTO struct {
	PromptID       int64  `json:"prompt_id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	VersionNo      string `json:"version_no"`
	Visibility     string `json:"visibility"`
	SensitiveLevel string `json:"sensitive_level"`
	CategoryNameZh string `json:"category_name_zh"`
	CategoryNameEn string `json:"category_name_en"`
	AddedAt        string `json:"added_at"`
}

type favoriteItemListResponse struct {
	Body struct {
		Items    []favoriteItemDTO `json:"items"`
		Page     int32             `json:"page"`
		PageSize int32             `json:"page_size"`
		Total    int64             `json:"total"`
	}
}

type listUserFavoritesInput struct {
	ID       int64 `path:"id" doc:"用户 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListUserFavorites(ctx context.Context, in *listUserFavoritesInput) (*favoriteListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListFavorites(ctx, v, in.ID, page, size)
	if err != nil {
		return nil, err
	}
	return buildFavoriteList(result), nil
}

type listMyFavoritesInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListMyFavorites(ctx context.Context, in *listMyFavoritesInput) (*favoriteListResponse, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListFavorites(ctx, domain.ViewerFor(actor), actor.ID, page, size)
	if err != nil {
		return nil, err
	}
	return buildFavoriteList(result), nil
}

func buildFavoriteList(result social.FavoritePage) *favoriteListResponse {
	out := &favoriteListResponse{}
	out.Body.Items = make([]favoriteDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, toFavoriteDTO(item))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out
}

type listFavoriteItemsInput struct {
	ID       int64 `path:"id" doc:"收藏夹 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListFavoriteItems(ctx context.Context, in *listFavoriteItemsInput) (*favoriteItemListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListItems(ctx, v, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &favoriteItemListResponse{}
	out.Body.Items = make([]favoriteItemDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, favoriteItemDTO{
			PromptID:       item.Prompt.ID,
			Title:          item.Prompt.Title,
			Description:    item.Prompt.Description,
			VersionNo:      item.Prompt.LatestVersion.String(),
			Visibility:     string(item.Prompt.Visibility),
			SensitiveLevel: string(item.Prompt.Sensitive),
			CategoryNameZh: item.CategoryNameZh,
			CategoryNameEn: item.CategoryNameEn,
			AddedAt:        formatTime(item.AddedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type createFavoriteInput struct {
	Body favoriteBody
}

func (s *Server) handleCreateFavorite(ctx context.Context, in *createFavoriteInput) (*favoriteWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	body, err := in.Body.toInput()
	if err != nil {
		return nil, err
	}
	created, err := s.deps.Social.CreateFavorite(ctx, domain.ViewerFor(actor), body)
	if err != nil {
		return nil, err
	}
	out := &favoriteWriteResponse{}
	out.Body.Favorite = toFavoriteDTO(created)
	return out, nil
}

type updateFavoriteInput struct {
	ID   int64 `path:"id" doc:"收藏夹 ID"`
	Body favoriteBody
}

func (s *Server) handleUpdateFavorite(ctx context.Context, in *updateFavoriteInput) (*favoriteWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	body, err := in.Body.toInput()
	if err != nil {
		return nil, err
	}
	updated, err := s.deps.Social.UpdateFavorite(ctx, domain.ViewerFor(actor), in.ID, body)
	if err != nil {
		return nil, err
	}
	out := &favoriteWriteResponse{}
	out.Body.Favorite = toFavoriteDTO(updated)
	return out, nil
}

type deleteFavoriteInput struct {
	ID int64 `path:"id" doc:"收藏夹 ID"`
}

func (s *Server) handleDeleteFavorite(ctx context.Context, in *deleteFavoriteInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.DeleteFavorite(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type favoriteItemInput struct {
	ID       int64 `path:"id" doc:"收藏夹 ID"`
	PromptID int64 `path:"prompt_id" doc:"提示词 ID"`
}

func (s *Server) handleAddFavoriteItem(ctx context.Context, in *favoriteItemInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.AddItem(ctx, domain.ViewerFor(actor), in.ID, in.PromptID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleRemoveFavoriteItem(ctx context.Context, in *favoriteItemInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.RemoveItem(ctx, domain.ViewerFor(actor), in.ID, in.PromptID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type isFavoritedInput struct {
	PromptID int64 `path:"prompt_id" doc:"提示词 ID"`
}

type isFavoritedResponse struct {
	Body struct {
		Favorited bool `json:"favorited"`
	}
}

func (s *Server) handleIsFavorited(ctx context.Context, in *isFavoritedInput) (*isFavoritedResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	favorited, err := s.deps.Social.IsFavorited(ctx, v, in.PromptID)
	if err != nil {
		return nil, err
	}
	out := &isFavoritedResponse{}
	out.Body.Favorited = favorited
	return out, nil
}

// ---------- 关注 ----------

type profileDTO struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Bio       string `json:"bio"`
	AvatarURL string `json:"avatar_url"`
	Followed  bool   `json:"followed"`
}

type profileResponse struct {
	Body struct {
		Profile profileDTO `json:"profile"`
	}
}

type userProfileInput struct {
	ID int64 `path:"id" doc:"用户 ID"`
}

func (s *Server) handleGetUserProfile(ctx context.Context, in *userProfileInput) (*profileResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := s.deps.Social.UserProfile(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &profileResponse{}
	out.Body.Profile = profileDTO{
		ID: profile.ID, Username: profile.Username, Bio: profile.Bio,
		AvatarURL: profile.AvatarURL, Followed: profile.Followed,
	}
	return out, nil
}

type followingListResponse struct {
	Body struct {
		Items    []profileDTO `json:"items"`
		Page     int32        `json:"page"`
		PageSize int32        `json:"page_size"`
		Total    int64        `json:"total"`
	}
}

type listFollowingInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListFollowing(ctx context.Context, in *listFollowingInput) (*followingListResponse, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListFollowing(ctx, domain.ViewerFor(actor), page, size)
	if err != nil {
		return nil, err
	}
	out := &followingListResponse{}
	out.Body.Items = make([]profileDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, profileDTO{
			ID: item.ID, Username: item.Username, Bio: item.Bio,
			AvatarURL: item.AvatarURL, Followed: item.Followed,
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type followUserInput struct {
	ID int64 `path:"id" doc:"被关注的用户 ID"`
}

func (s *Server) handleFollowUser(ctx context.Context, in *followUserInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.Follow(ctx, domain.ViewerFor(actor), in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleUnfollowUser(ctx context.Context, in *followUserInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.Unfollow(ctx, domain.ViewerFor(actor), in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

// ---------- 评论 ----------

type commentDTO struct {
	ID         int64  `json:"id"`
	PromptID   int64  `json:"prompt_id"`
	UserID     int64  `json:"user_id"`
	AuthorName string `json:"author_name"`
	ParentID   *int64 `json:"parent_id,omitempty"`
	Body       string `json:"body"`
	LikeCount  int32  `json:"like_count"`
	Liked      bool   `json:"liked" doc:"当前用户是否点过赞"`
	ReplyCount int64  `json:"reply_count"`
	CreatedAt  string `json:"created_at"`
}

func toCommentDTO(c social.Comment) commentDTO {
	return commentDTO{
		ID: c.ID, PromptID: c.PromptID, UserID: c.UserID, AuthorName: c.AuthorName,
		ParentID: c.ParentID, Body: c.Body, LikeCount: c.LikeCount,
		Liked: c.Liked, ReplyCount: c.ReplyCount, CreatedAt: formatTime(c.CreatedAt),
	}
}

type commentListResponse struct {
	Body struct {
		Items    []commentDTO `json:"items"`
		Page     int32        `json:"page"`
		PageSize int32        `json:"page_size"`
		Total    int64        `json:"total"`
	}
}

func buildCommentList(result social.CommentPage) *commentListResponse {
	out := &commentListResponse{}
	out.Body.Items = make([]commentDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, toCommentDTO(item))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out
}

type listCommentsInput struct {
	ID       int64 `path:"id" doc:"提示词 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListComments(ctx context.Context, in *listCommentsInput) (*commentListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListComments(ctx, v, in.ID, page, size)
	if err != nil {
		return nil, err
	}
	return buildCommentList(result), nil
}

type listRepliesInput struct {
	ID       int64 `path:"id" doc:"评论 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListReplies(ctx context.Context, in *listRepliesInput) (*commentListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListReplies(ctx, v, in.ID, page, size)
	if err != nil {
		return nil, err
	}
	return buildCommentList(result), nil
}

type createCommentBody struct {
	Body     string `json:"body" required:"true" minLength:"1" maxLength:"2000" doc:"评论内容"`
	ParentID *int64 `json:"parent_id,omitempty" minimum:"1" doc:"被回复的评论 ID"`
}

type createCommentInput struct {
	ID   int64 `path:"id" doc:"提示词 ID"`
	Body createCommentBody
}

type commentWriteResponse struct {
	Body struct {
		Comment commentDTO `json:"comment"`
	}
}

func (s *Server) handleCreateComment(ctx context.Context, in *createCommentInput) (*commentWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	created, err := s.deps.Social.CreateComment(ctx, domain.ViewerFor(actor), in.ID, in.Body.ParentID, in.Body.Body)
	if err != nil {
		return nil, err
	}
	out := &commentWriteResponse{}
	out.Body.Comment = toCommentDTO(created)
	return out, nil
}

type commentIDInput struct {
	ID int64 `path:"id" doc:"评论 ID"`
}

func (s *Server) handleDeleteComment(ctx context.Context, in *commentIDInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.DeleteComment(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleLikeComment(ctx context.Context, in *commentIDInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.LikeComment(ctx, domain.ViewerFor(actor), in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleUnlikeComment(ctx context.Context, in *commentIDInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.UnlikeComment(ctx, domain.ViewerFor(actor), in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

// ---------- 举报 ----------

type reportBody struct {
	ObjectType string `json:"object_type" required:"true" enum:"prompt,comment" doc:"被举报对象类型"`
	ObjectID   int64  `json:"object_id" required:"true" minimum:"1" doc:"被举报对象 ID"`
	Reason     string `json:"reason" required:"true" enum:"bad_content,illegal,porn,attack,privacy,spam,dangerous,other" doc:"举报原因（英文枚举键）"`
	Detail     string `json:"detail,omitempty" maxLength:"500" doc:"补充说明"`
}

type createReportInput struct {
	Body reportBody
}

type reportResponse struct {
	Body struct {
		ID         int64  `json:"id"`
		ObjectType string `json:"object_type"`
		ObjectID   int64  `json:"object_id"`
		Reason     string `json:"reason"`
		Created    bool   `json:"created"`
	}
}

func (s *Server) handleCreateReport(ctx context.Context, in *createReportInput) (*reportResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	objectType, ok := domain.ParseReportObjectType(in.Body.ObjectType)
	if !ok {
		return nil, apperr.New(apperr.CodeValidationFailed, "举报对象类型非法")
	}
	reason, ok := domain.ParseReportReason(in.Body.Reason)
	if !ok {
		return nil, apperr.New(apperr.CodeValidationFailed, "举报原因取值非法")
	}

	created, err := s.deps.Social.CreateReport(ctx, domain.ViewerFor(actor), social.ReportInput{
		ObjectType: objectType,
		ObjectID:   in.Body.ObjectID,
		Reason:     reason,
		Detail:     in.Body.Detail,
	})
	if err != nil {
		return nil, err
	}
	out := &reportResponse{}
	out.Body.ID = created.ID
	out.Body.ObjectType = string(created.ObjectType)
	out.Body.ObjectID = created.ObjectID
	// 举报原因以英文枚举键原样返回，前端按 error 之外的独立键做中英文案（C-31）。
	out.Body.Reason = string(created.Reason)
	out.Body.Created = true
	return out, nil
}

// ---------- 通知与公告 ----------

type notificationDTO struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind" enum:"announcement,comment_reply,library_question_deleted,system"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Link      string `json:"link"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

type notificationListResponse struct {
	Body struct {
		Items    []notificationDTO `json:"items"`
		Page     int32             `json:"page"`
		PageSize int32             `json:"page_size"`
		Total    int64             `json:"total"`
	}
}

type listNotificationsInput struct {
	Page     int32    `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32    `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
	Kinds    []string `query:"kind" doc:"按类型筛选，可重复；留空表示全部"`
}

func (s *Server) handleListNotifications(ctx context.Context, in *listNotificationsInput) (*notificationListResponse, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	kinds := make([]domain.NotificationKind, 0, len(in.Kinds))
	for _, raw := range in.Kinds {
		if raw == "" {
			continue
		}
		kind, ok := domain.ParseNotificationKind(raw)
		if !ok {
			return nil, apperr.New(apperr.CodeValidationFailed, "通知类型取值非法")
		}
		kinds = append(kinds, kind)
	}

	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListNotifications(ctx, domain.ViewerFor(actor), kinds, page, size)
	if err != nil {
		return nil, err
	}

	out := &notificationListResponse{}
	out.Body.Items = make([]notificationDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, notificationDTO{
			ID: item.ID, Kind: string(item.Kind), Title: item.Title, Body: item.Body,
			Link: item.Link, Read: item.Read, CreatedAt: formatTime(item.CreatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type unreadCountResponse struct {
	Body struct {
		Unread int64 `json:"unread"`
	}
}

func (s *Server) handleUnreadCount(ctx context.Context, _ *struct{}) (*unreadCountResponse, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.deps.Social.UnreadCount(ctx, domain.ViewerFor(actor))
	if err != nil {
		return nil, err
	}
	out := &unreadCountResponse{}
	out.Body.Unread = count
	return out, nil
}

func (s *Server) handleMarkAllNotificationsRead(ctx context.Context, _ *struct{}) (*struct{}, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.MarkAllNotificationsRead(ctx, domain.ViewerFor(actor)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleMarkAnnouncementsRead(ctx context.Context, _ *struct{}) (*struct{}, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.MarkAnnouncementsRead(ctx, domain.ViewerFor(actor)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type notificationIDInput struct {
	ID int64 `path:"id" doc:"通知 ID"`
}

func (s *Server) handleMarkNotificationRead(ctx context.Context, in *notificationIDInput) (*struct{}, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.MarkNotificationRead(ctx, domain.ViewerFor(actor), in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleDeleteNotification(ctx context.Context, in *notificationIDInput) (*struct{}, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Social.DeleteNotification(ctx, domain.ViewerFor(actor), in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type announcementDTO struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Read        bool   `json:"read"`
}

type announcementListResponse struct {
	Body struct {
		Items    []announcementDTO `json:"items"`
		Page     int32             `json:"page"`
		PageSize int32             `json:"page_size"`
		Total    int64             `json:"total"`
	}
}

type listAnnouncementsInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListAnnouncements(ctx context.Context, in *listAnnouncementsInput) (*announcementListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Social.ListAnnouncements(ctx, v, page, size)
	if err != nil {
		return nil, err
	}

	out := &announcementListResponse{}
	out.Body.Items = make([]announcementDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, announcementDTO{
			ID: item.ID, Title: item.Title, Body: item.Body,
			PublishedAt: formatTime(item.PublishedAt), Read: item.Read,
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}
