// 中文语言包。英文语言包以本对象推导出的类型为约束，缺失或多余的键都会导致类型错误。

const zh = {
  app: {
    title: 'Prompt Benchmark',
    tagline: '提示词能力边界评测平台',
  },
  nav: {
    home: '首页',
    language: '语言',
    theme: '主题',
    themeLight: '浅色',
    themeDark: '深色',
  },
  footer: {
    copyright: 'Prompt Benchmark',
  },
  home: {
    placeholder: '平台骨架已就绪，提示词列表将在后续阶段提供。',
  },
  notFound: {
    title: '页面不存在',
    description: '你访问的地址没有对应的页面，或该内容对当前身份不可见。',
    backHome: '返回首页',
  },
  // 错误码文案与后端错误码表一一对应，由 src/i18n/errors.spec.ts 校验。
  error: {
    not_found: '对象不存在',
    unauthorized: '未登录或会话已失效',
    forbidden: '权限不足',
    banned: '账户已被封禁',
    invalid_csrf: 'CSRF 校验未通过',
    rate_limited: '请求过于频繁，请稍后再试',
    bad_request: '请求无法解析',
    validation_failed: '请求字段校验未通过',
    invalid_state: '当前状态不允许该操作',
    invalid_category: '类别、能力或难度不合法',
    invalid_credentials: '邮箱或密码错误',
    email_taken: '邮箱已注册',
    email_not_verified: '邮箱未验证',
    already_verified: '邮箱已验证',
    invalid_code: '验证码错误或已过期',
    invalid_invite: '邀请码无效、已用完或已禁用',
    registration_closed: '注册已关闭',
    email_not_allowed: '邮箱后缀不在白名单内',
    mail_send_failed: '验证码邮件发送失败',
    mail_error: '测试邮件发送失败',
    oauth_not_configured: '未配置 GitHub OAuth',
    quota_exceeded: '超出配额',
    rating_exists: '已对该提示词与模型组合评过分',
    rating_edit_limit_exceeded: '评分修改次数已超限',
    rating_delete_limit_exceeded: '评分删除次数已超限',
    duplicate_model: '模型重复',
    model_in_use: '模型已被引用，不可删除',
    prompt_in_use: '提示词已被题库引用，不可彻底删除',
    permission_denied: '权限不足',
    secret_unavailable: '未配置主密钥，无法保存密钥类配置',
    bad_gateway: '上游服务请求失败',
    internal: '服务器内部错误',
    conflict: '请求与当前资源状态冲突',
    method_not_allowed: '请求方法不被允许',
    not_acceptable: '无法满足请求的响应格式',
    request_entity_too_large: '请求体超出大小上限',
    unsupported_media_type: '不支持的请求体类型',
    service_unavailable: '服务暂不可用',
    unprocessable_entity: '请求无法处理',
  },
}

export type MessageSchema = typeof zh
export default zh
