package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/library"
)

func (s *Server) registerLibraryRoutes() {
	// ---------- 题库 ----------

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/libraries",
		OperationID: "listMyLibraries", Summary: "我参与的私有题库", Tags: []string{"私有题库"},
	}, s.handleListMyLibraries)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/libraries/recycle-bin",
		OperationID: "listMyLibraryRecycleBin", Summary: "我回收站中的题库", Tags: []string{"私有题库"},
	}, s.handleListMyLibraryRecycleBin)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/libraries",
		OperationID: "createLibrary", Summary: "创建私有题库", Tags: []string{"私有题库"},
	}, s.handleCreateLibrary)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/libraries/{id}",
		OperationID: "getLibrary", Summary: "题库详情", Tags: []string{"私有题库"},
	}, s.handleGetLibrary)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/libraries/{id}",
		OperationID: "updateLibrary", Summary: "修改题库", Tags: []string{"私有题库"},
	}, s.handleUpdateLibrary)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/libraries/{id}",
		OperationID: "deleteLibrary", Summary: "删除题库", Tags: []string{"私有题库"},
	}, s.handleDeleteLibrary)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/libraries/{id}/restore",
		OperationID: "restoreLibrary", Summary: "恢复题库", Tags: []string{"私有题库"},
	}, s.handleRestoreLibrary)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/libraries/{id}/purge",
		OperationID: "purgeLibrary", Summary: "彻底删除题库", Tags: []string{"私有题库"},
	}, s.handlePurgeLibrary)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/libraries/{id}/export",
		OperationID: "exportLibrary", Summary: "导出题库为 Markdown", Tags: []string{"私有题库"},
	}, s.handleExportLibrary)

	// ---------- 成员 ----------

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/libraries/{id}/members",
		OperationID: "listLibraryMembers", Summary: "题库成员", Tags: []string{"私有题库"},
	}, s.handleListLibraryMembers)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/libraries/{id}/members",
		OperationID: "inviteLibraryMember", Summary: "邀请协作者", Tags: []string{"私有题库"},
	}, s.handleInviteLibraryMember)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/libraries/{id}/members/{user_id}",
		OperationID: "removeLibraryMember", Summary: "移除协作者", Tags: []string{"私有题库"},
	}, s.handleRemoveLibraryMember)

	// ---------- 题目 ----------

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/libraries/{id}/questions",
		OperationID: "listLibraryQuestions", Summary: "题库题目", Tags: []string{"私有题库"},
	}, s.handleListLibraryQuestions)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/libraries/{id}/questions",
		OperationID: "addLibraryQuestion", Summary: "添加题目", Tags: []string{"私有题库"},
	}, s.handleAddLibraryQuestion)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/libraries/{id}/questions/order",
		OperationID: "reorderLibraryQuestions", Summary: "调整题目顺序", Tags: []string{"私有题库"},
	}, s.handleReorderLibraryQuestions)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/libraries/{id}/recycle-bin",
		OperationID: "listLibraryRecycleBin", Summary: "题库回收站", Tags: []string{"私有题库"},
	}, s.handleListLibraryRecycleBin)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/library-questions/{id}",
		OperationID: "getLibraryQuestion", Summary: "题目详情", Tags: []string{"私有题库"},
	}, s.handleGetLibraryQuestion)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/library-questions/{id}",
		OperationID: "updateLibraryQuestion", Summary: "修改题目内容", Tags: []string{"私有题库"},
	}, s.handleUpdateLibraryQuestion)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/library-questions/{id}",
		OperationID: "deleteLibraryQuestion", Summary: "删除题目", Tags: []string{"私有题库"},
	}, s.handleDeleteLibraryQuestion)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/library-questions/{id}/restore",
		OperationID: "restoreLibraryQuestion", Summary: "恢复题目", Tags: []string{"私有题库"},
	}, s.handleRestoreLibraryQuestion)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/library-questions/{id}/purge",
		OperationID: "purgeLibraryQuestion", Summary: "彻底删除题目", Tags: []string{"私有题库"},
	}, s.handlePurgeLibraryQuestion)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/library-questions/{id}/tags",
		OperationID: "addLibraryQuestionTag", Summary: "给题目贴标签", Tags: []string{"私有题库"},
	}, s.handleAddLibraryQuestionTag)

	Register(s, "library_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/library-questions/{id}/tags",
		OperationID: "removeLibraryQuestionTag", Summary: "移除题目标签", Tags: []string{"私有题库"},
	}, s.handleRemoveLibraryQuestionTag)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/library-questions/{id}/revisions",
		OperationID: "listLibraryQuestionRevisions", Summary: "题目修改记录", Tags: []string{"私有题库"},
	}, s.handleListLibraryQuestionRevisions)
}

// ---------- 题库 ----------

type libraryBody struct {
	Name        string `json:"name" required:"true" minLength:"1" maxLength:"100" doc:"题库名称"`
	Description string `json:"description,omitempty" maxLength:"2000" doc:"题库描述"`
}

type libraryPatchBody struct {
	Name        *string `json:"name,omitempty" minLength:"1" maxLength:"100" doc:"题库名称，留空表示不修改"`
	Description *string `json:"description,omitempty" maxLength:"2000" doc:"题库描述，留空表示不修改"`
}

type libraryDTO struct {
	ID                int64  `json:"id"`
	OwnerID           int64  `json:"owner_id"`
	OwnerName         string `json:"owner_name,omitempty"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Role              string `json:"role" enum:"owner,collaborator" doc:"当前身份在题库中的角色"`
	MemberCount       int32  `json:"member_count"`
	QuestionCount     int32  `json:"question_count" doc:"占用容量的引用题数量，含回收站中的引用题"`
	LiveQuestionCount int64  `json:"live_question_count" doc:"可见题目数量，不含回收站中的题目"`
	QuestionLimit     int32  `json:"question_limit" doc:"题库题目容量上限"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

func toLibraryDTO(item library.Library, questionLimit int32) libraryDTO {
	return libraryDTO{
		ID:                item.Entity.ID,
		OwnerID:           item.Entity.OwnerID,
		OwnerName:         item.OwnerName,
		Name:              item.Entity.Name,
		Description:       item.Entity.Description,
		Role:              string(item.Role),
		MemberCount:       item.MemberCount,
		QuestionCount:     item.Entity.QuestionCount,
		LiveQuestionCount: item.LiveQuestionCount,
		QuestionLimit:     questionLimit,
		CreatedAt:         formatTime(item.Entity.CreatedAt),
		UpdatedAt:         formatTime(item.Entity.UpdatedAt),
	}
}

type libraryListResponse struct {
	Body struct {
		Items    []libraryDTO `json:"items"`
		Page     int32        `json:"page"`
		PageSize int32        `json:"page_size"`
		Total    int64        `json:"total"`
	}
}

type libraryWriteResponse struct {
	Body struct {
		Library libraryDTO `json:"library"`
	}
}

type listMyLibrariesInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListMyLibraries(ctx context.Context, in *listMyLibrariesInput) (*libraryListResponse, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Library.ListMyLibraries(ctx, domain.ViewerFor(actor), page, size)
	if err != nil {
		return nil, err
	}
	out := &libraryListResponse{}
	out.Body.Items = make([]libraryDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, toLibraryDTO(item, s.questionLimit))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type createLibraryInput struct {
	Body libraryBody
}

func (s *Server) handleCreateLibrary(ctx context.Context, in *createLibraryInput) (*libraryWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	created, err := s.deps.Library.CreateLibrary(ctx, domain.ViewerFor(actor), library.LibraryInput{
		Name:        in.Body.Name,
		Description: in.Body.Description,
	})
	if err != nil {
		return nil, err
	}
	out := &libraryWriteResponse{}
	out.Body.Library = toLibraryDTO(created, s.questionLimit)
	return out, nil
}

type libraryIDInput struct {
	ID int64 `path:"id" doc:"题库 ID"`
}

func (s *Server) handleGetLibrary(ctx context.Context, in *libraryIDInput) (*libraryWriteResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.deps.Library.GetLibrary(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &libraryWriteResponse{}
	out.Body.Library = toLibraryDTO(item, s.questionLimit)
	return out, nil
}

type updateLibraryInput struct {
	ID   int64 `path:"id" doc:"题库 ID"`
	Body libraryPatchBody
}

func (s *Server) handleUpdateLibrary(ctx context.Context, in *updateLibraryInput) (*libraryWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.deps.Library.UpdateLibrary(ctx, domain.ViewerFor(actor), in.ID, library.LibraryPatch{
		Name:        in.Body.Name,
		Description: in.Body.Description,
	})
	if err != nil {
		return nil, err
	}
	out := &libraryWriteResponse{}
	out.Body.Library = toLibraryDTO(updated, s.questionLimit)
	return out, nil
}

func (s *Server) handleDeleteLibrary(ctx context.Context, in *libraryIDInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.DeleteLibrary(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleRestoreLibrary(ctx context.Context, in *libraryIDInput) (*libraryWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	restored, err := s.deps.Library.RestoreLibrary(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	out := &libraryWriteResponse{}
	out.Body.Library = toLibraryDTO(restored, s.questionLimit)
	return out, nil
}

func (s *Server) handlePurgeLibrary(ctx context.Context, in *libraryIDInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.PurgeLibrary(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type libraryRecycleEntryDTO struct {
	LibraryID   int64  `json:"library_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DeletedBy   int64  `json:"deleted_by"`
	DeletedAt   string `json:"deleted_at"`
	ExpiresAt   string `json:"expires_at"`
}

type libraryRecycleListResponse struct {
	Body struct {
		Items []libraryRecycleEntryDTO `json:"items"`
	}
}

func (s *Server) handleListMyLibraryRecycleBin(ctx context.Context, _ *struct{}) (*libraryRecycleListResponse, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.deps.Library.ListMyLibraryRecycleBin(ctx, domain.ViewerFor(actor))
	if err != nil {
		return nil, err
	}
	out := &libraryRecycleListResponse{}
	out.Body.Items = make([]libraryRecycleEntryDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, libraryRecycleEntryDTO{
			LibraryID:   item.LibraryID,
			Name:        item.Name,
			Description: item.Description,
			DeletedBy:   item.DeletedBy,
			DeletedAt:   formatTime(item.DeletedAt),
			ExpiresAt:   formatTime(item.ExpiresAt),
		})
	}
	return out, nil
}

type exportResponse struct {
	Body struct {
		Filename string `json:"filename"`
		Content  string `json:"content" doc:"Markdown 全文，含 14 项要素"`
	}
}

func (s *Server) handleExportLibrary(ctx context.Context, in *libraryIDInput) (*exportResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.deps.Library.ExportLibrary(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &exportResponse{}
	out.Body.Filename = result.Filename
	out.Body.Content = result.Content
	return out, nil
}

// ---------- 成员 ----------

type memberDTO struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role" enum:"owner,collaborator"`
	InvitedBy *int64 `json:"invited_by,omitempty"`
	CreatedAt string `json:"created_at"`
}

type memberListResponse struct {
	Body struct {
		Items []memberDTO `json:"items"`
	}
}

func (s *Server) handleListLibraryMembers(ctx context.Context, in *libraryIDInput) (*memberListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.deps.Library.ListMembers(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &memberListResponse{}
	out.Body.Items = make([]memberDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, memberDTO{
			UserID: item.UserID, Username: item.Username, AvatarURL: item.AvatarURL,
			Role: string(item.Role), InvitedBy: item.InvitedBy, CreatedAt: formatTime(item.CreatedAt),
		})
	}
	return out, nil
}

type inviteMemberBody struct {
	Username string `json:"username" required:"true" minLength:"1" maxLength:"20" doc:"被邀请人的用户名"`
}

type inviteMemberInput struct {
	ID   int64 `path:"id" doc:"题库 ID"`
	Body inviteMemberBody
}

type memberWriteResponse struct {
	Body struct {
		Member memberDTO `json:"member"`
	}
}

func (s *Server) handleInviteLibraryMember(ctx context.Context, in *inviteMemberInput) (*memberWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	invited, err := s.deps.Library.InviteMember(ctx, domain.ViewerFor(actor), in.ID, in.Body.Username)
	if err != nil {
		return nil, err
	}
	out := &memberWriteResponse{}
	out.Body.Member = memberDTO{
		UserID: invited.UserID, Username: invited.Username, AvatarURL: invited.AvatarURL,
		Role: string(invited.Role), InvitedBy: invited.InvitedBy, CreatedAt: formatTime(invited.CreatedAt),
	}
	return out, nil
}

type removeMemberInput struct {
	ID     int64 `path:"id" doc:"题库 ID"`
	UserID int64 `path:"user_id" doc:"被移除的成员用户 ID"`
}

func (s *Server) handleRemoveLibraryMember(ctx context.Context, in *removeMemberInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.RemoveMember(ctx, domain.ViewerFor(actor), in.ID, in.UserID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

// ---------- 题目 ----------

type questionDTO struct {
	ID                  int64    `json:"id"`
	LibraryID           int64    `json:"library_id"`
	Kind                string   `json:"kind" enum:"reference,derived" doc:"引用题或派生题"`
	Title               string   `json:"title"`
	Body                string   `json:"body"`
	Description         string   `json:"description"`
	PurposeID           *int64   `json:"purpose_id,omitempty"`
	PurposeNameZh       string   `json:"purpose_name_zh"`
	PurposeNameEn       string   `json:"purpose_name_en"`
	CapabilityID        *int64   `json:"capability_id,omitempty"`
	CapabilityNameZh    string   `json:"capability_name_zh"`
	CapabilityNameEn    string   `json:"capability_name_en"`
	DifficultyID        *int64   `json:"difficulty_id,omitempty"`
	SensitiveLevel      string   `json:"sensitive_level"`
	SortOrder           int32    `json:"sort_order"`
	Tags                []string `json:"tags"`
	SourcePromptID      *int64   `json:"source_prompt_id,omitempty"`
	SourceVersionNo     string   `json:"source_version_no,omitempty"`
	SourceAvailable     bool     `json:"source_available" doc:"来源提示词是否仍然存在"`
	SourceMajorModified bool     `json:"source_major_modified" doc:"来源提示词是否发生过重大修改"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

func toQuestionDTO(q library.Question) questionDTO {
	return questionDTO{
		ID: q.ID, LibraryID: q.LibraryID, Kind: string(q.Kind), Title: q.Title, Body: q.Body,
		Description: q.Description, PurposeID: q.PurposeID, PurposeNameZh: q.PurposeNameZh,
		PurposeNameEn: q.PurposeNameEn, CapabilityID: q.CapabilityID,
		CapabilityNameZh: q.CapabilityNameZh, CapabilityNameEn: q.CapabilityNameEn,
		DifficultyID:   q.DifficultyID,
		SensitiveLevel: string(q.Sensitive), SortOrder: q.SortOrder, Tags: q.Tags,
		SourcePromptID: q.SourcePromptID, SourceVersionNo: q.SourceVersionNo,
		SourceAvailable: q.SourceAvailable, SourceMajorModified: q.SourceMajorModified,
		CreatedAt: formatTime(q.CreatedAt), UpdatedAt: formatTime(q.UpdatedAt),
	}
}

type questionListResponse struct {
	Body struct {
		Items    []questionDTO `json:"items"`
		Page     int32         `json:"page"`
		PageSize int32         `json:"page_size"`
		Total    int64         `json:"total"`
	}
}

type questionWriteResponse struct {
	Body struct {
		Question questionDTO `json:"question"`
	}
}

type listQuestionsInput struct {
	ID       int64 `path:"id" doc:"题库 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数"`
}

func (s *Server) handleListLibraryQuestions(ctx context.Context, in *listQuestionsInput) (*questionListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Library.ListQuestions(ctx, v, in.ID, page, size)
	if err != nil {
		return nil, err
	}
	out := &questionListResponse{}
	out.Body.Items = make([]questionDTO, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Items = append(out.Body.Items, toQuestionDTO(item))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type addQuestionBody struct {
	PromptID     *int64   `json:"prompt_id,omitempty" minimum:"1" doc:"引用题的来源提示词 ID；填写时忽略正文"`
	Title        string   `json:"title,omitempty" maxLength:"200" doc:"派生题标题"`
	Body         string   `json:"body,omitempty" maxLength:"200000" doc:"派生题正文"`
	Description  string   `json:"description,omitempty" maxLength:"2000" doc:"题目说明"`
	PurposeID    *int64   `json:"purpose_id,omitempty" minimum:"1" doc:"派生题考察目的"`
	CapabilityID *int64   `json:"capability_id,omitempty" minimum:"1" doc:"派生题考察维度"`
	DifficultyID *int64   `json:"difficulty_id,omitempty" minimum:"1" doc:"派生题难度"`
	Sensitive    string   `json:"sensitive_level,omitempty" enum:"low,medium,high" doc:"敏感级别，默认 low"`
	Tags         []string `json:"tags,omitempty" doc:"题目标签"`
}

type addQuestionInput struct {
	ID   int64 `path:"id" doc:"题库 ID"`
	Body addQuestionBody
}

func (s *Server) handleAddLibraryQuestion(ctx context.Context, in *addQuestionInput) (*questionWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	sensitive, err := parseSensitive(in.Body.Sensitive)
	if err != nil {
		return nil, err
	}
	created, err := s.deps.Library.AddQuestion(ctx, domain.ViewerFor(actor), in.ID, library.AddQuestionInput{
		PromptID:     in.Body.PromptID,
		Title:        in.Body.Title,
		Body:         in.Body.Body,
		Description:  in.Body.Description,
		PurposeID:    in.Body.PurposeID,
		CapabilityID: in.Body.CapabilityID,
		DifficultyID: in.Body.DifficultyID,
		Sensitive:    sensitive,
		Tags:         in.Body.Tags,
	})
	if err != nil {
		return nil, err
	}
	out := &questionWriteResponse{}
	out.Body.Question = toQuestionDTO(created)
	return out, nil
}

type updateQuestionBody struct {
	Title        string `json:"title" required:"true" minLength:"1" maxLength:"200" doc:"题目标题"`
	Body         string `json:"body" required:"true" minLength:"1" maxLength:"200000" doc:"题目正文"`
	Description  string `json:"description,omitempty" maxLength:"2000" doc:"题目说明"`
	PurposeID    *int64 `json:"purpose_id,omitempty" minimum:"1" doc:"题目考察目的"`
	CapabilityID *int64 `json:"capability_id,omitempty" minimum:"1" doc:"题目考察维度"`
	DifficultyID *int64 `json:"difficulty_id,omitempty" minimum:"1" doc:"题目难度"`
	Sensitive    string `json:"sensitive_level,omitempty" enum:"low,medium,high" doc:"敏感级别"`
	Note         string `json:"change_note,omitempty" maxLength:"500" doc:"修改说明"`
}

type updateQuestionInput struct {
	ID   int64 `path:"id" doc:"题目 ID"`
	Body updateQuestionBody
}

func (s *Server) handleUpdateLibraryQuestion(ctx context.Context, in *updateQuestionInput) (*questionWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	sensitive, err := parseSensitive(in.Body.Sensitive)
	if err != nil {
		return nil, err
	}
	updated, err := s.deps.Library.UpdateQuestion(ctx, domain.ViewerFor(actor), in.ID, library.UpdateQuestionInput{
		Title:        in.Body.Title,
		Body:         in.Body.Body,
		Description:  in.Body.Description,
		PurposeID:    in.Body.PurposeID,
		CapabilityID: in.Body.CapabilityID,
		DifficultyID: in.Body.DifficultyID,
		Sensitive:    sensitive,
		Note:         in.Body.Note,
	})
	if err != nil {
		return nil, err
	}
	out := &questionWriteResponse{}
	out.Body.Question = toQuestionDTO(updated)
	return out, nil
}

type questionIDInput struct {
	ID int64 `path:"id" doc:"题目 ID"`
}

func (s *Server) handleGetLibraryQuestion(ctx context.Context, in *questionIDInput) (*questionWriteResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	question, err := s.deps.Library.GetQuestion(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &questionWriteResponse{}
	out.Body.Question = toQuestionDTO(question)
	return out, nil
}

func (s *Server) handleDeleteLibraryQuestion(ctx context.Context, in *questionIDInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.DeleteQuestion(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleRestoreLibraryQuestion(ctx context.Context, in *questionIDInput) (*questionWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	restored, err := s.deps.Library.RestoreQuestion(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	out := &questionWriteResponse{}
	out.Body.Question = toQuestionDTO(restored)
	return out, nil
}

func (s *Server) handlePurgeLibraryQuestion(ctx context.Context, in *questionIDInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.PurgeQuestion(ctx, domain.ViewerFor(actor), in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type reorderBody struct {
	IDs []int64 `json:"ids" required:"true" minItems:"1" doc:"题目 ID 序列，按目标顺序排列"`
}

type reorderInput struct {
	ID   int64 `path:"id" doc:"题库 ID"`
	Body reorderBody
}

func (s *Server) handleReorderLibraryQuestions(ctx context.Context, in *reorderInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.ReorderQuestions(ctx, domain.ViewerFor(actor), in.ID, in.Body.IDs); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type tagBody struct {
	Tag string `json:"tag" required:"true" minLength:"1" maxLength:"50" doc:"题目标签"`
}

type questionTagInput struct {
	ID   int64 `path:"id" doc:"题目 ID"`
	Body tagBody
}

func (s *Server) handleAddLibraryQuestionTag(ctx context.Context, in *questionTagInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.AddTag(ctx, domain.ViewerFor(actor), in.ID, in.Body.Tag); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

// removeTagInput 以查询参数接收标签：标签是自由文本，放进路径段会与转义规则纠缠。
type removeTagInput struct {
	ID  int64  `path:"id" doc:"题目 ID"`
	Tag string `query:"tag" required:"true" minLength:"1" maxLength:"50" doc:"要移除的标签"`
}

func (s *Server) handleRemoveLibraryQuestionTag(ctx context.Context, in *removeTagInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Library.RemoveTag(ctx, domain.ViewerFor(actor), in.ID, in.Tag); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type recycleEntryDTO struct {
	Question  questionDTO `json:"question"`
	DeletedBy int64       `json:"deleted_by"`
	DeletedAt string      `json:"deleted_at"`
	ExpiresAt string      `json:"expires_at"`
}

type recycleListResponse struct {
	Body struct {
		Items []recycleEntryDTO `json:"items"`
	}
}

func (s *Server) handleListLibraryRecycleBin(ctx context.Context, in *libraryIDInput) (*recycleListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.deps.Library.ListRecycleBin(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &recycleListResponse{}
	out.Body.Items = make([]recycleEntryDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, recycleEntryDTO{
			Question:  toQuestionDTO(item.Question),
			DeletedBy: item.DeletedBy,
			DeletedAt: formatTime(item.DeletedAt),
			ExpiresAt: formatTime(item.ExpiresAt),
		})
	}
	return out, nil
}

type revisionDTO struct {
	ID         int64  `json:"id"`
	EditorID   int64  `json:"editor_id"`
	EditorName string `json:"editor_name"`
	Note       string `json:"note"`
	Before     string `json:"before" doc:"修改前取值（JSON 文本）"`
	After      string `json:"after" doc:"修改后取值（JSON 文本）"`
	CreatedAt  string `json:"created_at"`
}

type revisionListResponse struct {
	Body struct {
		Items []revisionDTO `json:"items"`
	}
}

func (s *Server) handleListLibraryQuestionRevisions(ctx context.Context, in *questionIDInput) (*revisionListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.deps.Library.ListRevisions(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &revisionListResponse{}
	out.Body.Items = make([]revisionDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, revisionDTO{
			ID: item.ID, EditorID: item.EditorID, EditorName: item.EditorName, Note: item.Note,
			Before: string(item.Before), After: string(item.After), CreatedAt: formatTime(item.CreatedAt),
		})
	}
	return out, nil
}

// parseSensitive 解析可选敏感级别，空值表示沿用默认。
func parseSensitive(raw string) (domain.SensitiveLevel, error) {
	if raw == "" {
		return "", nil
	}
	level, ok := domain.ParseSensitiveLevel(raw)
	if !ok {
		return "", apperr.New(apperr.CodeValidationFailed, "敏感级别取值非法")
	}
	return level, nil
}
