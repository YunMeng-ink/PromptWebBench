// Package showcase 承载模型结果展示区：用户在打分时提交的模型输出文本与结果图片，
// 以及按「模型分类 × 模型」聚合的公开展示。
//
// 图片本体存放在外部图床（internal/imgbed），数据库只保存外链；
// 管理员可删除违规图片（软删除，展示区与详情页立即不可见）。
package showcase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/imgbed"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// 图片与模型输出文本的限制。
const (
	// MaxImageBytes 是单张图片的大小上限（8 MB），与图床接口的常见上限留出余量。
	MaxImageBytes = 8 << 20
	// MaxImagesPerRating 是单条评分可上传的图片张数上限。
	MaxImagesPerRating = 4
	// MaxOutputRunes 是单条模型输出文本的长度上限（字符数）。
	MaxOutputRunes = 8000
)

// 允许的图片 MIME 类型；文件类型以浏览器声明的 Content-Type 为准，
// 图床服务端仍会按文件真实内容（magic bytes）二次识别。
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"image/bmp":  ".bmp",
	"image/tiff": ".tiff",
}

type Deps struct {
	Pool   *pgxpool.Pool
	Imgbed *imgbed.Client
	Logger *slog.Logger
}

type Service struct {
	pool   *pgxpool.Pool
	imgbed *imgbed.Client
	logger *slog.Logger
}

func New(deps Deps) *Service {
	return &Service{pool: deps.Pool, imgbed: deps.Imgbed, logger: deps.Logger}
}

// UploadInput 是一次附图上传的输入。
type UploadInput struct {
	PromptID int64
	ModelID  int64
	// File 是 multipart 中的图片文件。
	File io.Reader
	// ContentType 是浏览器声明的图片 MIME 类型。
	ContentType string
	// Filename 是原始文件名（可为空，服务端按类型补全）。
	Filename string
	// Size 是文件字节数，由 multipart 解析层提供。
	Size int64
}

// OutputInput 是一次模型输出文本提交的输入。
type OutputInput struct {
	PromptID int64
	ModelID  int64
	Content  string
}

// Image 是一张已入库的评分附图。
type Image struct {
	ID        int64
	RatingID  int64
	UserID    int64
	PromptID  int64
	ModelID   int64
	URL       string
	Filename  string
	CreatedAt string
}

// Output 是一条已入库的模型输出文本。
type Output struct {
	ID        int64
	RatingID  int64
	UserID    int64
	PromptID  int64
	ModelID   int64
	Content   string
	CreatedAt string
}

// Group 是展示区中一个模型的素材聚合。
type Group struct {
	ModelID      int64
	ModelName    string
	ProviderKey  string
	ProviderName string
	ImageCount   int64
	OutputCount  int64
	// LastContributedAt 是该模型最近一次素材提交时间（图床外链时间以外的平台时间）。
	LastContributedAt string
}

// Entry 是展示区模型详情中的一条素材：图片与文本输出混排。
type Entry struct {
	Kind        string // "image" 或 "output"
	ID          int64
	RatingID    int64
	UserID      int64
	URL         string
	Content     string
	PromptID    int64
	PromptTitle string
	Username    string
	CreatedAt   string
}

// AdminImage 是管理员图片列表中的一条记录。
type AdminImage struct {
	ID          int64
	URL         string
	Filename    string
	PromptID    int64
	PromptTitle string
	ModelID     int64
	ModelName   string
	Uploader    string
	CreatedAt   string
}

// AdminOutput 是管理员模型输出列表中的一条记录。
type AdminOutput struct {
	ID          int64
	Content     string
	PromptID    int64
	PromptTitle string
	ModelID     int64
	ModelName   string
	Uploader    string
	CreatedAt   string
}

// 上传前先把图片送到图床，再把外链写入 rating_images。
// 入库与评分的绑定关系在事务内判定：评分必须存在且属于当前用户。
func (s *Service) UploadImage(ctx context.Context, actor domain.User, in UploadInput) (Image, error) {
	if !actor.CanWriteContent() {
		return Image{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if in.Size <= 0 || in.Size > MaxImageBytes {
		return Image{}, apperr.New(apperr.CodeValidationFailed, "图片大小须在 8 MB 以内")
	}
	ext, ok := allowedImageTypes[strings.ToLower(strings.TrimSpace(in.ContentType))]
	if !ok {
		return Image{}, apperr.New(apperr.CodeValidationFailed, "仅支持 jpg、png、webp、gif、bmp、tiff 图片")
	}

	// 先在事务内校验评分与数量上限，再外呼图床，避免把注定失败的文件送出去。
	var ratingID int64
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		rating, err := s.ownRating(ctx, q, actor.ID, in.PromptID, in.ModelID)
		if err != nil {
			return err
		}
		ratingID = rating.ID
		images, err := q.ListRatingImagesByRating(ctx, ratingID)
		if err != nil {
			return fmt.Errorf("查询评分附图: %w", err)
		}
		if len(images) >= MaxImagesPerRating {
			return apperr.Errorf(apperr.CodeQuotaExceeded, "单条评分最多上传 %d 张图片", MaxImagesPerRating)
		}
		return nil
	})
	if err != nil {
		return Image{}, err
	}

	filename := strings.TrimSpace(in.Filename)
	if filename == "" || !strings.Contains(filename, ".") {
		filename = "result" + ext
	}

	result, err := s.imgbed.Upload(ctx, filename, in.ContentType, in.File)
	if err != nil {
		s.logger.Error("图床上传失败", slog.String("error", err.Error()))
		return Image{}, apperr.New(apperr.CodeBadGateway, "图床上传失败，请稍后重试")
	}

	var image Image
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		// 锁住评分行后再计数并写入：并发上传会在锁上排队，
		// 后到的事务读到前一笔已提交的图片，张数上限不会被并发突破。
		if _, err := q.LockRating(ctx, ratingID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidState, "评分已被删除，无法上传结果")
			}
			return fmt.Errorf("锁定评分: %w", err)
		}
		images, err := q.ListRatingImagesByRating(ctx, ratingID)
		if err != nil {
			return fmt.Errorf("查询评分附图: %w", err)
		}
		if len(images) >= MaxImagesPerRating {
			return apperr.Errorf(apperr.CodeQuotaExceeded, "单条评分最多上传 %d 张图片", MaxImagesPerRating)
		}
		row, err := q.InsertRatingImage(ctx, repo.InsertRatingImageParams{
			RatingID: ratingID,
			UserID:   actor.ID,
			PromptID: in.PromptID,
			ModelID:  in.ModelID,
			Url:      result.URL,
			Filename: result.Filename,
		})
		if err != nil {
			return fmt.Errorf("写入评分附图: %w", err)
		}
		image = imageFromRow(row)
		return nil
	})
	if err != nil {
		return Image{}, err
	}
	return image, nil
}

// SubmitOutput 把模型输出文本写入 rating_outputs，绑定当前用户的评分。
func (s *Service) SubmitOutput(ctx context.Context, actor domain.User, in OutputInput) (Output, error) {
	if !actor.CanWriteContent() {
		return Output{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return Output{}, apperr.New(apperr.CodeValidationFailed, "模型输出内容不能为空")
	}
	if len([]rune(content)) > MaxOutputRunes {
		return Output{}, apperr.Errorf(apperr.CodeValidationFailed, "模型输出不能超过 %d 字", MaxOutputRunes)
	}

	var output Output
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		rating, err := s.ownRating(ctx, q, actor.ID, in.PromptID, in.ModelID)
		if err != nil {
			return err
		}
		row, err := q.InsertRatingOutput(ctx, repo.InsertRatingOutputParams{
			RatingID: rating.ID,
			UserID:   actor.ID,
			PromptID: in.PromptID,
			ModelID:  in.ModelID,
			Content:  content,
		})
		if err != nil {
			return fmt.Errorf("写入模型输出: %w", err)
		}
		output = outputFromRow(row)
		return nil
	})
	if err != nil {
		return Output{}, err
	}
	return output, nil
}

// Groups 返回展示区的模型聚合：按提供厂商分组、模型名排序，
// 只列出已有素材的模型。
func (s *Service) Groups(ctx context.Context) ([]Group, error) {
	rows, err := repo.New(s.pool).ListShowcaseGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询展示区模型聚合: %w", err)
	}
	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		last := row.LastImageAt
		if row.LastOutputAt.Valid && (!last.Valid || row.LastOutputAt.Time.After(last.Time)) {
			last = row.LastOutputAt
		}
		groups = append(groups, Group{
			ModelID:           row.ModelID,
			ModelName:         row.ModelName,
			ProviderKey:       row.ProviderKey,
			ProviderName:      row.ProviderName,
			ImageCount:        row.ImageCount,
			OutputCount:       row.OutputCount,
			LastContributedAt: formatTimestamp(last),
		})
	}
	return groups, nil
}

// ModelEntries 返回某个模型的素材分页。
func (s *Service) ModelEntries(ctx context.Context, modelID int64, page, pageSize int32) ([]Entry, int64, error) {
	q := repo.New(s.pool)
	total, err := q.CountShowcaseEntries(ctx, modelID)
	if err != nil {
		return nil, 0, fmt.Errorf("统计模型素材: %w", err)
	}
	rows, err := q.ListShowcaseEntries(ctx, repo.ListShowcaseEntriesParams{
		ModelID: modelID,
		Limit:   pageSize,
		Offset:  (page - 1) * pageSize,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("查询模型素材: %w", err)
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, Entry{
			Kind:        row.Kind,
			ID:          row.EntryID,
			RatingID:    row.RatingID,
			UserID:      row.UserID,
			URL:         row.Url,
			Content:     row.Content,
			PromptID:    row.PromptID,
			PromptTitle: row.PromptTitle,
			Username:    row.Username,
			CreatedAt:   formatTimestamp(row.CreatedAt),
		})
	}
	return entries, total, nil
}

// AdminImages 返回管理员图片列表。
func (s *Service) AdminImages(ctx context.Context, page, pageSize int32) ([]AdminImage, int64, error) {
	q := repo.New(s.pool)
	total, err := q.CountAdminRatingImages(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("统计图片数量: %w", err)
	}
	rows, err := q.ListAdminRatingImages(ctx, repo.ListAdminRatingImagesParams{
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("查询图片列表: %w", err)
	}
	items := make([]AdminImage, 0, len(rows))
	for _, row := range rows {
		items = append(items, AdminImage{
			ID:          row.ID,
			URL:         row.Url,
			Filename:    row.Filename,
			PromptID:    row.PromptID,
			PromptTitle: row.PromptTitle,
			ModelID:     row.ModelID,
			ModelName:   row.ModelName,
			Uploader:    row.Uploader,
			CreatedAt:   formatTimestamp(row.CreatedAt),
		})
	}
	return items, total, nil
}

// AdminOutputs 返回管理员模型输出列表。
func (s *Service) AdminOutputs(ctx context.Context, page, pageSize int32) ([]AdminOutput, int64, error) {
	q := repo.New(s.pool)
	total, err := q.CountAdminRatingOutputs(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("统计模型输出数量: %w", err)
	}
	rows, err := q.ListAdminRatingOutputs(ctx, repo.ListAdminRatingOutputsParams{
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("查询模型输出列表: %w", err)
	}
	items := make([]AdminOutput, 0, len(rows))
	for _, row := range rows {
		items = append(items, AdminOutput{
			ID:          row.ID,
			Content:     row.Content,
			PromptID:    row.PromptID,
			PromptTitle: row.PromptTitle,
			ModelID:     row.ModelID,
			ModelName:   row.ModelName,
			Uploader:    row.Uploader,
			CreatedAt:   formatTimestamp(row.CreatedAt),
		})
	}
	return items, total, nil
}

// AdminDeleteImage 由管理员删除一张违规图片（软删除）。
// 图床侧的文件无法由平台删除，平台侧删除后外链不再出现在任何展示位置。
func (s *Service) AdminDeleteImage(ctx context.Context, actor domain.User, imageID int64, ip string) error {
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		image, err := q.GetRatingImage(ctx, imageID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询评分附图: %w", err)
		}
		n, err := q.SoftDeleteRatingImage(ctx, repo.SoftDeleteRatingImageParams{
			ID:        imageID,
			DeletedBy: repo.Int8(actor.ID),
		})
		if err != nil {
			return fmt.Errorf("删除评分附图: %w", err)
		}
		if n == 0 {
			return apperr.New(apperr.CodeNotFound, "")
		}
		recorder := oplog.NewRecorder()
		return recorder.Record(ctx, q, oplog.Entry{
			OperatorID:   actor.ID,
			OperatorName: actor.Username,
			Action:       oplog.ActionShowcaseImageDelete,
			ObjectType:   "rating_image",
			ObjectID:     image.ID,
			Before:       map[string]any{"url": image.Url, "filename": image.Filename},
			Note:         "管理员删除违规图片",
			IP:           ip,
		})
	})
}

// AdminDeleteOutput 由管理员删除一条违规模型输出（软删除）。
func (s *Service) AdminDeleteOutput(ctx context.Context, actor domain.User, outputID int64, ip string) error {
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		output, err := q.GetRatingOutput(ctx, outputID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询模型输出: %w", err)
		}
		n, err := q.SoftDeleteRatingOutput(ctx, repo.SoftDeleteRatingOutputParams{
			ID:        outputID,
			DeletedBy: repo.Int8(actor.ID),
		})
		if err != nil {
			return fmt.Errorf("删除模型输出: %w", err)
		}
		if n == 0 {
			return apperr.New(apperr.CodeNotFound, "")
		}
		recorder := oplog.NewRecorder()
		return recorder.Record(ctx, q, oplog.Entry{
			OperatorID:   actor.ID,
			OperatorName: actor.Username,
			Action:       oplog.ActionShowcaseOutputDelete,
			ObjectType:   "rating_output",
			ObjectID:     output.ID,
			Before:       map[string]any{"content": output.Content},
			Note:         "管理员删除违规模型输出",
			IP:           ip,
		})
	})
}

// ownRating 取当前用户在某「提示词 × 模型」上的活跃评分，作为附图与输出的归属。
func (s *Service) ownRating(ctx context.Context, q *repo.Queries, userID, promptID, modelID int64) (repo.Rating, error) {
	rating, err := q.GetRating(ctx, repo.GetRatingParams{
		UserID:   userID,
		PromptID: promptID,
		ModelID:  modelID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Rating{}, apperr.New(apperr.CodeInvalidState, "请先对该模型提交评分，再上传结果")
		}
		return repo.Rating{}, fmt.Errorf("查询评分: %w", err)
	}
	return rating, nil
}

func imageFromRow(row repo.RatingImage) Image {
	return Image{
		ID:        row.ID,
		RatingID:  row.RatingID,
		UserID:    row.UserID,
		PromptID:  row.PromptID,
		ModelID:   row.ModelID,
		URL:       row.Url,
		Filename:  row.Filename,
		CreatedAt: formatTimestamp(row.CreatedAt),
	}
}

func outputFromRow(row repo.RatingOutput) Output {
	return Output{
		ID:        row.ID,
		RatingID:  row.RatingID,
		UserID:    row.UserID,
		PromptID:  row.PromptID,
		ModelID:   row.ModelID,
		Content:   row.Content,
		CreatedAt: formatTimestamp(row.CreatedAt),
	}
}

// formatTimestamp 把 timestamptz 行值转为统一的时间字符串；空值返回空串。
func formatTimestamp(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}
