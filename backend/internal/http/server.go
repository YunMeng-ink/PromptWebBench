package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/metrics"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/admin"
	"github.com/promptbench/backend/internal/service/auth"
	"github.com/promptbench/backend/internal/service/prompt"
	"github.com/promptbench/backend/internal/service/rating"
	"github.com/promptbench/backend/internal/service/stats"
)

// APIPrefix 是全部业务端点的前缀（G-17）。
const APIPrefix = "/api/v1"

// Deps 是接口层的构造依赖，全部由 main 手工注入。
type Deps struct {
	Env     config.Env
	Logger  *slog.Logger
	Pool    *pgxpool.Pool
	Config  *config.Store
	Metrics *metrics.Metrics
	Repo    *repo.Queries

	Auth   *auth.Service
	Admin  *admin.Service
	Prompt *prompt.Service
	Rating *rating.Service
	Stats  *stats.Service
	Signer *security.Signer
}

// RouteRecord 记录一个已登记路由及其限流规则，供启动日志与自检使用。
type RouteRecord struct {
	Method string
	Path   string
	Rule   string
}

// Server 组装 Echo、Huma 与横切中间件。
type Server struct {
	deps    Deps
	echo    *echo.Echo
	api     huma.API
	limiter *Limiter
	signer  *security.Signer
	trusted []netip.Prefix
	routes  []RouteRecord
}

func New(deps Deps) (*Server, error) {
	if deps.Logger == nil || deps.Pool == nil || deps.Config == nil || deps.Metrics == nil {
		return nil, fmt.Errorf("接口层依赖不完整：Logger、Pool、Config、Metrics 均不可为空")
	}
	if deps.Auth == nil || deps.Admin == nil || deps.Signer == nil {
		return nil, fmt.Errorf("接口层依赖不完整：Auth、Admin、Signer 均不可为空")
	}
	if deps.Prompt == nil || deps.Rating == nil || deps.Repo == nil {
		return nil, fmt.Errorf("接口层依赖不完整：Prompt、Rating、Repo 均不可为空")
	}
	if deps.Stats == nil {
		return nil, fmt.Errorf("接口层依赖不完整：Stats 不可为空")
	}
	trusted, err := ParseTrustedProxies(deps.Env.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("解析 TRUSTED_PROXIES: %w", err)
	}
	if len(deps.Env.CORSAllowOrigins) == 0 {
		return nil, fmt.Errorf("CORS_ALLOW_ORIGINS 至少需要一个来源")
	}

	e := echo.New()
	e.Logger = deps.Logger
	e.HTTPErrorHandler = EchoErrorHandler(deps.Logger)

	s := &Server{
		deps:    deps,
		echo:    e,
		limiter: NewLimiter(trusted, DefaultRules()),
		signer:  deps.Signer,
		trusted: trusted,
	}
	if deps.Env.AuthRateLimit > 0 {
		if err := s.limiter.OverrideLimit("auth", deps.Env.AuthRateLimit); err != nil {
			return nil, err
		}
	}
	if deps.Env.RateLimit > 0 {
		if err := s.limiter.OverrideAll(deps.Env.RateLimit); err != nil {
			return nil, err
		}
	}
	// 限流的用户维度取自 Authenticate 中间件写入的身份；Authenticate 先于
	// 各操作的限流中间件执行。
	s.limiter.SetUserResolver(func(ctx huma.Context) (int64, bool) {
		state, ok := ctx.Context().Value(viewerContextKey{}).(viewerState)
		if !ok || state.user == nil {
			return 0, false
		}
		return state.user.ID, true
	})
	e.Use(
		EchoRecover(),
		EchoRequestID(),
		EchoRequestLogger(deps.Logger),
		EchoSecure(),
		EchoCORS(deps.Env.CORSAllowOrigins),
		EchoBodyLimit(),
		EchoMetrics(deps.Metrics),
		EchoCSRF(deps.Env.CookieSecure),
	)

	// 错误模型必须在创建 API 之前覆盖，否则后续全部端点的错误形状不受控。
	InstallErrorModel()
	s.api = humaecho.New(e, humaConfig())
	s.api.UseMiddleware(s.Authenticate)

	s.registerInfraRoutes()
	s.registerAuthRoutes()
	s.registerMeRoutes()
	s.registerAdminAccountRoutes()
	s.registerPromptRoutes()
	s.registerRatingRoutes()
	s.registerStatsRoutes()
	s.registerDictionaryRoutes()
	return s, nil
}

// humaConfig 构造 Huma 配置。
// 错误响应组件无需手工登记：Huma 在注册每个操作时会对 huma.NewError 的返回值取类型，
// 由于 NewError 已被 InstallErrorModel 覆盖为返回 *APIError，规范中的错误模型
// 自动等于 {error:{code,message}}，与实现同源。
func humaConfig() huma.Config {
	cfg := huma.DefaultConfig("Prompt Benchmark API", "0.1.0")
	cfg.OpenAPI.Info.Description = "提示词能力边界评测平台接口。全部错误响应为 {error:{code,message}}。"
	cfg.OpenAPIPath = APIPrefix + "/openapi"
	cfg.DocsPath = APIPrefix + "/docs"
	cfg.SchemasPath = APIPrefix + "/schemas"
	return cfg
}

// Echo 暴露底层 Echo 实例，供测试注入与静态资源挂载。
func (s *Server) Echo() *echo.Echo { return s.echo }

// API 暴露 Huma 实例，供各业务模块注册端点。
func (s *Server) API() huma.API { return s.api }

// Limiter 暴露限流器，供认证层注入用户解析函数。
func (s *Server) Limiter() *Limiter { return s.limiter }

// Handler 返回可挂载到 http.Server 的处理器。
func (s *Server) Handler() http.Handler { return s.echo }

// Dependencies 返回构造依赖，供业务模块取用。
func (s *Server) Dependencies() Deps { return s.deps }

// RegisteredRoutes 返回登记过的业务路由。
func (s *Server) RegisteredRoutes() []RouteRecord {
	out := make([]RouteRecord, len(s.routes))
	copy(out, s.routes)
	return out
}

// Register 注册一个 Huma 操作，并强制声明限流规则。
// 写方法未声明规则、或声明的规则不在集中声明表中，一律 panic，
// 使「漏挂限流」在启动阶段就暴露，不能进入运行期。
func Register[I, O any](s *Server, rule string, op huma.Operation, handler func(context.Context, *I) (*O, error)) {
	s.mustValidateRule(op.Method, op.Path, rule)
	if rule != "" {
		op.Middlewares = append(op.Middlewares, s.limiter.Middleware(s.api, rule))
	}
	huma.Register(s.api, op, func(ctx context.Context, input *I) (*O, error) {
		out, err := handler(ctx, input)
		if err != nil {
			// 服务层错误在此统一转为线格式，业务代码无需感知响应形状。
			return nil, Translate(err)
		}
		return out, nil
	})
	s.routes = append(s.routes, RouteRecord{Method: op.Method, Path: op.Path, Rule: rule})
}

func (s *Server) mustValidateRule(method, path, rule string) {
	if rule == "" {
		if isWriteMethod(method) {
			panic(fmt.Errorf("写路由 %s %s 未声明限流规则", method, path))
		}
		return
	}
	if _, ok := s.limiter.lookupRule(rule); !ok {
		panic(fmt.Errorf("路由 %s %s 声明的限流规则 %q 未在集中声明表中登记", method, path, rule))
	}
}

func (s *Server) registerInfraRoutes() {
	s.echo.GET(APIPrefix+"/health", s.handleHealth)
	s.echo.GET(APIPrefix+"/metrics", s.handleMetrics)
}

func (s *Server) handleHealth(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	if err := s.deps.Pool.Ping(ctx); err != nil {
		s.deps.Logger.Error("健康检查失败", slog.String("error", err.Error()))
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// handleMetrics 仅允许来自回环地址的抓取（G-02）。
func (s *Server) handleMetrics(c *echo.Context) error {
	if !s.metricsReachable(c) {
		return NewError(apperr.CodeNotFound, "")
	}
	s.deps.Metrics.Handler().ServeHTTP(c.Response(), c.Request())
	return nil
}

// metricsReachable 只允许直连回环地址抓取指标（G-02）。
// 携带转发头说明请求经过了中间层：未配置受信反代时一律拒绝，避免伪造 X-Forwarded-For
// 把指标端点暴露出去；配置了受信反代时，只有其转发的回环客户端才允许抓取。
func (s *Server) metricsReachable(c *echo.Context) bool {
	remote, err := netip.ParseAddr(hostOf(c.Request().RemoteAddr))
	if err != nil || !remote.IsLoopback() {
		return false
	}

	forwarded := c.Request().Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return true
	}
	if len(s.trusted) == 0 {
		return false
	}
	client, err := netip.ParseAddr(ClientIP(c.Request().RemoteAddr, forwarded, s.trusted))
	return err == nil && client.IsLoopback()
}
