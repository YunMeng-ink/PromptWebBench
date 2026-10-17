package admin_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/mail/mailtest"
	"github.com/promptbench/backend/internal/modelsdev"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/admin"
	"github.com/promptbench/backend/internal/service/prompt"
	"github.com/promptbench/backend/internal/service/social"
	"github.com/promptbench/backend/internal/service/stats"
)

// fullHarness 是阶段 6 用例的测试环境：真实数据库，加上后台服务真正依赖的
// 提示词域、社交域、统计域、SMTP 与 models.dev 替身。
//
// 不替换任何被测试实现：导入用例走真实 HTTP 客户端与解析代码，
// 测试邮件走真实 SMTP 会话，提示词写入走提示词域的真实事务。
type fullHarness struct {
	*harness

	pool   *pgxpool.Pool
	config *config.Store
	repo   *repo.Queries
	prompt *prompt.Service
	social *social.Service
	stats  *stats.Service

	mailer  *mailtest.Server
	catalog *catalogServer
}

// catalogServer 是 models.dev 的替身，可切换响应体与状态码。
type catalogServer struct {
	server *httptest.Server

	mu     sync.Mutex
	status int
	body   string
	hits   int
}

func newCatalogServer(t *testing.T) *catalogServer {
	t.Helper()
	server := &catalogServer{status: http.StatusOK, body: catalogFixture}
	server.server = httptest.NewServer(http.HandlerFunc(server.serve))
	t.Cleanup(server.server.Close)
	return server
}

func (c *catalogServer) url() string { return c.server.URL }

func (c *catalogServer) serve(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits++
	w.WriteHeader(c.status)
	_, _ = w.Write([]byte(c.body))
}

func (c *catalogServer) set(status int, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = status
	c.body = body
}

func (c *catalogServer) requests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits
}

// catalogFixture 覆盖字段齐全、缺 modalities、缺 limit 三种条目形态（需求 §13.3）。
const catalogFixture = `{
  "alpha": {
    "id": "alpha",
    "name": "Alpha Lab",
    "models": {
      "alpha-one": {
        "id": "alpha-one",
        "name": "Alpha One",
        "description": "第一个模型",
        "release_date": "2025-01-31",
        "limit": {"context": 128000},
        "modalities": {"input": ["text", "image"]}
      },
      "alpha-two": {"id": "alpha-two", "name": "Alpha Two", "limit": {"context": 32000}}
    }
  },
  "beta": {
    "id": "beta",
    "name": "Beta Lab",
    "models": {
      "beta-one": {
        "id": "beta-one",
        "name": "Beta One",
        "release_date": "2024-12-01",
        "modalities": {"input": ["text", "audio"]}
      }
    }
  }
}`

const testMasterKey = "phase6-master-key-0123456789abcdef"

func newFullHarness(t *testing.T) *fullHarness {
	t.Helper()

	pool := pgtest.MigratedDatabase(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfgStore := config.NewStore(pool, config.DefaultCacheTTL)
	t.Cleanup(cfgStore.Close)
	if err := cfgStore.Refresh(context.Background()); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	mailServer := mailtest.New(t)
	catalog := newCatalogServer(t)

	statsService := stats.New(stats.Deps{Pool: pool, Config: cfgStore, Logger: logger})
	promptService := prompt.New(prompt.Deps{
		Pool: pool, Config: cfgStore, Logger: logger, Stats: statsService,
	})
	socialService := social.New(social.Deps{Pool: pool, Config: cfgStore, Logger: logger})
	adminService := admin.New(admin.Deps{
		Pool:       pool,
		Config:     cfgStore,
		Logger:     logger,
		Secrets:    security.NewSecretBox(testMasterKey),
		Mail:       mail.Config{Host: mailServer.Host(), Port: mailServer.Port(), From: "noreply@example.test"},
		ModelsDev:  modelsdev.NewClient(catalog.url()),
		Prompts:    promptService,
		Moderation: socialService,
		Stats:      statsService,
	})

	return &fullHarness{
		harness: &harness{pool: pool, service: adminService},
		pool:    pool,
		config:  cfgStore,
		repo:    repo.New(pool),
		prompt:  promptService,
		social:  socialService,
		stats:   statsService,
		mailer:  mailServer,
		catalog: catalog,
	}
}

// seedLibraryWithReference 建一个题库并把某条提示词作为引用题加入，
// 用于验证「被题库引用的提示词不可彻底删除」。
func (h *fullHarness) seedLibraryWithReference(t *testing.T, ownerID, promptID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var libraryID int64
	if err := h.pool.QueryRow(ctx, `
		INSERT INTO libraries (owner_id, name, description, question_count)
		VALUES ($1, '引用测试题库', '', 1)
		RETURNING id`, ownerID).Scan(&libraryID); err != nil {
		t.Fatalf("写入题库失败: %v", err)
	}
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO library_questions (library_id, kind, source_prompt_id, description)
		VALUES ($1, 'reference', $2, '')`, libraryID, promptID); err != nil {
		t.Fatalf("写入引用题失败: %v", err)
	}
	return libraryID
}

// seedRating 直接写入一条评分及其变更日志之外的最小字段，使统计与后台列表有数据可读。
func (h *fullHarness) seedRating(t *testing.T, userID, promptID, modelID int64, score int16) {
	t.Helper()
	ctx := context.Background()
	var versionID int64
	if err := h.pool.QueryRow(ctx,
		`SELECT current_version_id FROM prompts WHERE id = $1`, promptID).Scan(&versionID); err != nil {
		t.Fatalf("读取提示词当前版本失败: %v", err)
	}
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO ratings (user_id, prompt_id, prompt_version_id, model_id, score)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, promptID, versionID, modelID, score); err != nil {
		t.Fatalf("写入评分失败: %v", err)
	}
}

// seedModel 写入一个模型，供评分与模型管理用例使用。
func (h *fullHarness) seedModel(t *testing.T, name, modelKey, providerKey string) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(), `
		INSERT INTO models (name, model_key, provider_key, provider_name)
		VALUES ($1, $2, $3, '测试厂商')
		RETURNING id`, name, modelKey, providerKey).Scan(&id); err != nil {
		t.Fatalf("写入模型失败: %v", err)
	}
	return id
}

// seedRecommendTag 写入一条待确认的推荐标签。
func (h *fullHarness) seedRecommendTag(t *testing.T, promptID, modelID int64, kind, state string) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(), `
		INSERT INTO recommend_tags (
			prompt_id, model_id, scope, kind, state, label_zh, label_en, confidence, reason
		) VALUES ($1, $2, 'prompt_model', $3, $4, '中文文案', 'English label', 'high', '测试理由')
		RETURNING id`, promptID, modelID, kind, state).Scan(&id); err != nil {
		t.Fatalf("写入推荐标签失败: %v", err)
	}
	return id
}

func (h *fullHarness) scalar(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := h.pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("读取标量失败: %v", err)
	}
	return value
}

func (h *fullHarness) scalarString(t *testing.T, query string, args ...any) string {
	t.Helper()
	var value string
	if err := h.pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("读取标量失败: %v", err)
	}
	return value
}
