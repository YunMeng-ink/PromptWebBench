package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/rating"
)

func (s *Server) registerRatingRoutes() {
	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/ratings/mine",
		OperationID: "listMyRatings", Summary: "我的评分", Tags: []string{"评分"},
	}, s.handleListMyRatings)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/prompts/{id}/ratings",
		OperationID: "listPromptRatings", Summary: "提示词的模型评分汇总", Tags: []string{"评分"},
	}, s.handleListPromptRatings)

	Register(s, "rating_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/prompts/{id}/ratings",
		OperationID: "createRating", Summary: "提交评分", Tags: []string{"评分"},
	}, s.handleCreateRating)

	Register(s, "rating_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/prompts/{id}/ratings",
		OperationID: "updateRating", Summary: "修改评分", Tags: []string{"评分"},
	}, s.handleUpdateRating)

	Register(s, "rating_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/prompts/{id}/ratings/{model_id}",
		OperationID: "deleteRating", Summary: "删除评分", Tags: []string{"评分"},
	}, s.handleDeleteRating)
}

type ratingBody struct {
	ModelID int64 `json:"model_id" required:"true" minimum:"1" doc:"被评分的模型"`
	Score   int16 `json:"score" required:"true" minimum:"1" maximum:"5" doc:"1-5 锚定评分"`
}

type ratingResponse struct {
	Body struct {
		ID        int64  `json:"id"`
		PromptID  int64  `json:"prompt_id"`
		ModelID   int64  `json:"model_id"`
		Score     int16  `json:"score"`
		EditCount int32  `json:"edit_count"`
		MaxEdits  int32  `json:"max_edits" doc:"单条评分的修改次数上限"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
}

func toRatingResponse(r domain.Rating, maxEdits int32) *ratingResponse {
	out := &ratingResponse{}
	out.Body.ID = r.ID
	out.Body.PromptID = r.PromptID
	out.Body.ModelID = r.ModelID
	out.Body.Score = r.Score
	out.Body.EditCount = r.EditCount
	out.Body.MaxEdits = maxEdits
	out.Body.CreatedAt = formatTime(r.CreatedAt)
	out.Body.UpdatedAt = formatTime(r.UpdatedAt)
	return out
}

type createRatingInput struct {
	ID   int64 `path:"id" doc:"提示词 ID"`
	Body ratingBody
}

func (s *Server) handleCreateRating(ctx context.Context, in *createRatingInput) (*ratingResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	created, err := s.deps.Rating.Submit(ctx, actor, rating.SubmitInput{
		PromptID: in.ID,
		ModelID:  in.Body.ModelID,
		Score:    int(in.Body.Score),
	}, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	maxEdits, err := s.deps.Rating.MaxEdits(ctx)
	if err != nil {
		return nil, err
	}
	return toRatingResponse(created, maxEdits), nil
}

type updateRatingInput struct {
	ID   int64 `path:"id" doc:"提示词 ID"`
	Body ratingBody
}

func (s *Server) handleUpdateRating(ctx context.Context, in *updateRatingInput) (*ratingResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.deps.Rating.Update(ctx, actor, rating.SubmitInput{
		PromptID: in.ID,
		ModelID:  in.Body.ModelID,
		Score:    int(in.Body.Score),
	}, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	maxEdits, err := s.deps.Rating.MaxEdits(ctx)
	if err != nil {
		return nil, err
	}
	return toRatingResponse(updated, maxEdits), nil
}

type deleteRatingInput struct {
	ID      int64 `path:"id" doc:"提示词 ID"`
	ModelID int64 `path:"model_id" doc:"模型 ID"`
}

func (s *Server) handleDeleteRating(ctx context.Context, in *deleteRatingInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Rating.Delete(ctx, actor, in.ID, in.ModelID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type listMyRatingsInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数，默认 20，上限 100"`
}

type myRatingDTO struct {
	ID          int64  `json:"id"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	ModelID     int64  `json:"model_id"`
	ModelName   string `json:"model_name"`
	Score       int16  `json:"score"`
	EditCount   int32  `json:"edit_count"`
	CreatedAt   string `json:"created_at"`
}

type myRatingListResponse struct {
	Body struct {
		Items    []myRatingDTO `json:"items"`
		Page     int32         `json:"page"`
		PageSize int32         `json:"page_size"`
		Total    int64         `json:"total"`
	}
}

func (s *Server) handleListMyRatings(ctx context.Context, in *listMyRatingsInput) (*myRatingListResponse, error) {
	actor, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	ratings, total, err := s.deps.Rating.ListMine(ctx, actor.ID, page, size)
	if err != nil {
		return nil, err
	}
	out := &myRatingListResponse{}
	out.Body.Items = make([]myRatingDTO, 0, len(ratings))
	for _, r := range ratings {
		out.Body.Items = append(out.Body.Items, myRatingDTO{
			ID:          r.ID,
			PromptID:    r.PromptID,
			PromptTitle: r.PromptTitle,
			ModelID:     r.ModelID,
			ModelName:   r.ModelName,
			Score:       r.Score,
			EditCount:   r.EditCount,
			CreatedAt:   formatTime(r.CreatedAt),
		})
	}
	out.Body.Page = page
	out.Body.PageSize = size
	out.Body.Total = total
	return out, nil
}

type promptRatingDTO struct {
	ModelID     int64   `json:"model_id"`
	ModelName   string  `json:"model_name"`
	RatingCount int64   `json:"rating_count"`
	UserCount   int64   `json:"user_count"`
	MeanScore   float64 `json:"mean_score" doc:"原始平均分；阶段 3 之前按真实评分计算"`
	// MyScore 为当前用户在该模型上的评分；未评分为 nil。
	MyScore *int16 `json:"my_score,omitempty"`
	// MyEditCount 与 MyScore 同步出现，供前端展示剩余修改次数。
	MyEditCount *int32 `json:"my_edit_count,omitempty"`
}

type promptRatingListResponse struct {
	Body struct {
		Items []promptRatingDTO `json:"items"`
	}
}

type listPromptRatingsInput struct {
	ID int64 `path:"id" doc:"提示词 ID"`
}

func (s *Server) handleListPromptRatings(ctx context.Context, in *listPromptRatingsInput) (*promptRatingListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	// 可见性先行：不可见的提示词不返回任何评分信息。
	if _, err := s.deps.Prompt.Get(ctx, v, in.ID); err != nil {
		return nil, err
	}

	items, err := s.deps.Rating.ListByPrompt(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	out := &promptRatingListResponse{}
	out.Body.Items = make([]promptRatingDTO, 0, len(items))
	for _, item := range items {
		dto := promptRatingDTO{
			ModelID:     item.ModelID,
			ModelName:   item.ModelName,
			RatingCount: item.RatingCount,
			UserCount:   item.UserCount,
			MeanScore:   item.MeanScore,
		}
		if item.MyScore != nil {
			dto.MyScore = item.MyScore
			dto.MyEditCount = item.MyEditCount
		}
		out.Body.Items = append(out.Body.Items, dto)
	}
	return out, nil
}
