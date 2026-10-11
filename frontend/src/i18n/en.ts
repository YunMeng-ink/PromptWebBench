// 英文语言包。类型受 MessageSchema 约束，与中文键一一对应，缺键与多键都会编译失败。

import type { MessageSchema } from './zh'

const en: MessageSchema = {
  app: {
    title: 'Prompt Benchmark',
    tagline: 'Benchmarking the capability boundaries of language models',
  },
  nav: {
    home: 'Home',
    language: 'Language',
    theme: 'Theme',
    themeLight: 'Light',
    themeDark: 'Dark',
  },
  footer: {
    copyright: 'Prompt Benchmark',
  },
  home: {
    placeholder: 'The platform skeleton is ready. Prompt listings arrive in a later stage.',
  },
  notFound: {
    title: 'Page not found',
    description: 'There is no page at this address, or the content is not visible to you.',
    backHome: 'Back to home',
  },
  error: {
    not_found: 'Not found',
    unauthorized: 'Not signed in, or the session has expired',
    forbidden: 'Insufficient permission',
    banned: 'This account is banned',
    invalid_csrf: 'CSRF validation failed',
    rate_limited: 'Too many requests, please try again later',
    bad_request: 'The request could not be parsed',
    validation_failed: 'Request validation failed',
    invalid_state: 'The current state does not allow this operation',
    invalid_category: 'Invalid category, ability, or difficulty',
    invalid_credentials: 'Incorrect email or password',
    email_taken: 'This email is already registered',
    email_not_verified: 'Email not verified',
    already_verified: 'Email already verified',
    invalid_code: 'The verification code is wrong or expired',
    invalid_invite: 'The invite code is invalid, exhausted, or disabled',
    registration_closed: 'Registration is closed',
    email_not_allowed: 'This email domain is not on the allowlist',
    mail_send_failed: 'Failed to send the verification email',
    mail_error: 'Failed to send the test email',
    oauth_not_configured: 'GitHub OAuth is not configured',
    quota_exceeded: 'Quota exceeded',
    rating_exists: 'You have already rated this prompt and model pair',
    rating_edit_limit_exceeded: 'Rating edit limit exceeded',
    rating_delete_limit_exceeded: 'Rating delete limit exceeded',
    duplicate_model: 'Duplicate model',
    model_in_use: 'This model is referenced and cannot be deleted',
    prompt_in_use: 'This prompt is referenced by a library and cannot be purged',
    permission_denied: 'Insufficient permission',
    secret_unavailable: 'No master key configured, cannot store secret settings',
    bad_gateway: 'The upstream service request failed',
    internal: 'Internal server error',
    conflict: 'The request conflicts with the current resource state',
    method_not_allowed: 'Method not allowed',
    not_acceptable: 'Cannot satisfy the requested response format',
    request_entity_too_large: 'Request body exceeds the size limit',
    unsupported_media_type: 'Unsupported request body type',
    service_unavailable: 'Service temporarily unavailable',
    unprocessable_entity: 'The request cannot be processed',
  },
}

export default en
