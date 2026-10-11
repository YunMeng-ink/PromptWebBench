package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/promptbench/backend/internal/metrics"
)

// BodyLimitBytes 是请求体上限（开发方案 §5.1）。
const BodyLimitBytes = 1 << 20

// EchoErrorHandler 把任何到达全局错误处理器的错误规范为 {error:{code,message}}。
// Echo 中间件（CSRF、限流回退、路由未匹配等）产生的错误走这里，
// Huma 侧的错误由 humaecho 直接写响应，不经此处。
func EchoErrorHandler(logger *slog.Logger) echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		var apiErr *APIError
		switch {
		case errors.As(err, &apiErr):
			writeAPIError(c, apiErr)
		default:
			var sc echo.HTTPStatusCoder
			status := http.StatusInternalServerError
			if errors.As(err, &sc) && sc.StatusCode() != 0 {
				status = sc.StatusCode()
			}
			if status >= http.StatusInternalServerError {
				logger.Error("请求处理失败",
					slog.String("request_id", c.Response().Header().Get(echo.HeaderXRequestID)),
					slog.String("path", c.Request().URL.Path),
					slog.String("error", err.Error()),
				)
			}
			writeAPIError(c, NewError(codeForStatus(status), ""))
		}
	}
}

func writeAPIError(c *echo.Context, apiErr *APIError) {
	if err := c.JSON(apiErr.GetStatus(), apiErr); err != nil {
		c.Logger().Error("写出错误响应失败", "error", err)
	}
}

// codeForStatus 把状态码映射为错误码；未知状态码按 4xx/5xx 归类。
func codeForStatus(status int) Code {
	if code, ok := statusToCode[status]; ok {
		return code
	}
	if status >= http.StatusInternalServerError {
		return CodeInternal
	}
	return CodeUnprocessableEntity
}

// CSRFErrorHandler 让 CSRF 校验失败返回 invalid_csrf。
func CSRFErrorHandler(c *echo.Context, err error) error {
	return NewError(CodeInvalidCSRF, "")
}

// isBearerRequest 判定是否为 API Token 通道。Bearer 通道豁免 CSRF（开发方案 §5.1）。
func isBearerRequest(c *echo.Context) bool {
	auth := c.Request().Header.Get(echo.HeaderAuthorization)
	return strings.HasPrefix(auth, "Bearer ")
}

// EchoRequestID 生成并回写 X-Request-Id。
func EchoRequestID() echo.MiddlewareFunc {
	return middleware.RequestIDWithConfig(middleware.RequestIDConfig{TargetHeader: echo.HeaderXRequestID})
}

// EchoRecover 捕获 panic 并转为 500。panic 与其堆栈由 Echo 经 Context 日志写出。
func EchoRecover() echo.MiddlewareFunc {
	return middleware.Recover()
}

// EchoRequestLogger 输出结构化访问日志。
func EchoRequestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogLatency:   true,
		LogMethod:    true,
		LogURIPath:   true,
		LogRoutePath: true,
		LogRequestID: true,
		LogRemoteIP:  true,
		LogStatus:    true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			attrs := []slog.Attr{
				slog.String("request_id", v.RequestID),
				slog.String("method", v.Method),
				slog.String("path", v.URIPath),
				slog.String("route", v.RoutePath),
				slog.String("ip", v.RemoteIP),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
			}
			if v.Error != nil {
				attrs = append(attrs, slog.String("error", v.Error.Error()))
				logger.LogAttrs(c.Request().Context(), slog.LevelError, "请求失败", attrs...)
				return nil
			}
			logger.LogAttrs(c.Request().Context(), slog.LevelInfo, "请求", attrs...)
			return nil
		},
	})
}

// EchoSecure 设置安全响应头。
func EchoSecure() echo.MiddlewareFunc {
	return middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "SAMEORIGIN",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		ContentSecurityPolicy: "",
	})
}

// EchoCORS 允许前端开发服务器跨域携带 Cookie。
func EchoCORS(allowOrigins []string) echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-CSRF-Token", echo.HeaderXRequestID},
		ExposeHeaders:    []string{echo.HeaderXRequestID},
		AllowCredentials: true,
		MaxAge:           600,
	})
}

// EchoCSRF 装载 CSRF 双提交 Cookie 校验，Bearer 通道豁免。
func EchoCSRF(cookieSecure bool) echo.MiddlewareFunc {
	return middleware.CSRFWithConfig(middleware.CSRFConfig{
		Skipper:      isBearerRequest,
		CookieName:   "pb_csrf",
		CookiePath:   "/",
		CookieSecure: cookieSecure,
		ErrorHandler: CSRFErrorHandler,
	})
}

// EchoBodyLimit 限制请求体大小。
func EchoBodyLimit() echo.MiddlewareFunc {
	return middleware.BodyLimit(BodyLimitBytes)
}

// EchoMetrics 记录请求计数与耗时。
func EchoMetrics(m *metrics.Metrics) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)
			res, _ := echo.UnwrapResponse(c.Response())
			status := http.StatusOK
			if res != nil && res.Status != 0 {
				status = res.Status
			}
			if err != nil {
				var sc echo.HTTPStatusCoder
				if errors.As(err, &sc) && sc.StatusCode() != 0 {
					status = sc.StatusCode()
				} else {
					status = http.StatusInternalServerError
				}
			}
			route := c.Path()
			if route == "" {
				route = "unmatched"
			}
			m.HTTPRequests.WithLabelValues(c.Request().Method, route, strconv.Itoa(status)).Inc()
			m.HTTPDuration.WithLabelValues(c.Request().Method, route).Observe(time.Since(start).Seconds())
			return err
		}
	}
}
