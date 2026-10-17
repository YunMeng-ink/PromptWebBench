// 管理后台的接口封装：统计仪表盘、提示词、模型、用户、标签、举报、
// 邀请码、操作日志、公告、回收站与系统设置（需求第 17 章）。
// 类型与后端 DTO 一一对应，枚举值保持英文键，界面按 admin.* 的语言包呈现。

import { api } from './client'
import type { Page, PromptVersion, Visibility } from './prompt'

export type ReportStatus = 'pending' | 'ignored' | 'resolved'
export type ReportObjectType = 'prompt' | 'comment'
export type ModelImportAction = 'import' | 'overwrite' | 'skip'
export type ConfigKind = 'int' | 'float' | 'bool' | 'string' | 'enum' | 'string_list'
export type RecommendTagState = 'pending' | 'confirmed' | 'ignored' | 'expired' | 'review'
export type RecycleObjectType = 'prompt' | 'library' | 'library_question'
export type AnnouncementState = 'draft' | 'live' | 'withdrawn'

/** RECOMMEND_TAG_STATES 是推荐标签筛选器的取值顺序。 */
export const RECOMMEND_TAG_STATES: RecommendTagState[] = [
  'pending',
  'review',
  'confirmed',
  'ignored',
  'expired',
]

/** RECOMMEND_TAG_KINDS 是推荐标签类型的取值顺序。 */
export const RECOMMEND_TAG_KINDS = [
  'obsolete',
  'suspected_obsolete',
  'saturation',
  'suspected_saturation',
] as const

/** INPUT_MODALITIES 是输入模态的候选项（需求 §13.3）。 */
export const INPUT_MODALITIES = ['text', 'image', 'audio', 'video', 'pdf'] as const

export interface DashboardPrompt {
  id: number
  title: string
  visibility: Visibility
  comment_count: number
  favorite_count: number
  created_at: string
}

export interface Dashboard {
  user_total: number
  user_today: number
  user_role_user: number
  user_role_developer: number
  user_banned: number
  prompt_total: number
  prompt_public: number
  prompt_private: number
  prompt_developer: number
  prompt_today: number
  rating_total: number
  comment_total: number
  report_pending: number
  recycle_total: number
  model_total: number
  invite_total: number
  recommend_pending: number
  hot_prompts: DashboardPrompt[]
  recent_prompts: DashboardPrompt[]
}

export interface AdminPrompt {
  id: number
  title: string
  description: string
  purpose_id: number
  purpose_name_zh: string
  purpose_name_en: string
  capability_id: number
  capability_name_zh: string
  capability_name_en: string
  difficulty_id?: number
  visibility: Visibility
  sensitive_level: string
  uploader_id: number
  uploader_name: string
  version_no: string
  major_modified: boolean
  is_high_risk: boolean
  comment_count: number
  favorite_count: number
  deleted: boolean
  created_at: string
  updated_at: string
}

export interface AdminPromptDetail {
  prompt: AdminPrompt
  body: string
  tags: {
    id: number
    slug: string
    name_zh: string
    name_en: string
    color: string
    kind: 'theme' | 'form'
  }[]
  versions: PromptVersion[]
}

export interface AdminRating {
  id: number
  user_id: number
  user_name: string
  model_id: number
  model_name: string
  score: number
  edit_count: number
  deleted: boolean
  created_at: string
  updated_at: string
}

export interface AdminComment {
  id: number
  user_id: number
  author_name: string
  parent_id?: number
  body: string
  like_count: number
  deleted: boolean
  created_at: string
}

export interface AdminRecommendTag {
  id: number
  prompt_id: number
  prompt_title: string
  model_id: number
  model_name: string
  scope: string
  kind: string
  state: RecommendTagState
  label_zh: string
  label_en: string
  confidence: string
  reason: string
  snapshot?: unknown
  triggered_at: string
  reviewed_at?: string
}

export interface AdminModel {
  id: number
  name: string
  model_key: string
  provider_key: string
  provider_name: string
  input_modalities: string[]
  aliases: string[]
  description: string
  release_date: string
  context_length?: number
  is_enabled: boolean
  capability_ids: number[]
  note: string
  import_source: string
  rating_count: number
  prompt_count: number
  deleted: boolean
  created_at: string
  updated_at: string
}

export interface AdminModelInput {
  name: string
  model_key: string
  provider_key: string
  provider_name: string
  input_modalities: string[]
  aliases: string[]
  description: string
  release_date: string
  context_length?: number
  is_enabled: boolean
  capability_ids: number[]
  note: string
}

export interface AdminModelImportEntry {
  provider_id: string
  provider_name: string
  model_id: string
  name: string
  description: string
  release_date: string
  input_modalities: string[]
  context_length?: number
  duplicate?: { id: number; name: string; provider_name: string }
}

export interface AdminModelImportResult {
  total: number
  created: number
  overwritten: number
  skipped: number
}

export interface AdminModelImportLog {
  id: number
  model_id?: number
  action: ModelImportAction
  provider_key: string
  model_key: string
  payload?: unknown
  operator_name: string
  created_at: string
}

export interface AdminModelPrompt {
  id: number
  title: string
  visibility: Visibility
  created_at: string
}

export interface AdminModelStat {
  prompt_id: number
  prompt_title: string
  history_rating_count: number
  history_adjusted: number
  recent_rating_count: number
  recent_adjusted: number
  computed_at: string
}

export interface AdminModelSnapshot {
  id: number
  model_id: number
  source: string
  raw?: unknown
  created_at: string
}

export interface AdminUser {
  id: number
  username: string
  email: string
  role: string
  status: string
  bio: string
  avatar_url: string
  email_verified: boolean
  is_system: boolean
  banned_reason: string
  prompt_count: number
  has_password: boolean
  created_at: string
  updated_at: string
}

export interface AdminUserRating {
  id: number
  prompt_id: number
  prompt_title: string
  model_id: number
  model_name: string
  score: number
  edit_count: number
  deleted: boolean
  created_at: string
}

export interface AdminUserComment {
  id: number
  prompt_id: number
  prompt_title: string
  parent_id?: number
  body: string
  deleted: boolean
  created_at: string
}

export interface AdminUserLibrary {
  id: number
  name: string
  description: string
  question_count: number
  deleted: boolean
  created_at: string
  updated_at: string
}

export interface AdminUserReport {
  id: number
  object_type: ReportObjectType
  object_id: number
  reason: string
  status: ReportStatus
  detail: string
  created_at: string
}

export interface AdminTag {
  id: number
  slug: string
  name_zh: string
  name_en: string
  color: string
  description: string
  kind: 'theme' | 'form'
  priority: number
  is_enabled: boolean
  usage_count: number
  created_at: string
  updated_at: string
}

export interface AdminTagInput {
  slug: string
  name_zh: string
  name_en: string
  color: string
  description: string
  kind: 'theme' | 'form'
  priority: number
  is_enabled: boolean
}

export interface AdminReport {
  id: number
  reporter_id: number
  reporter_name: string
  object_type: ReportObjectType
  object_id: number
  reason: string
  detail: string
  status: ReportStatus
  handler_id?: number
  handler_name: string
  handle_note: string
  handled_at?: string
  created_at: string
  prompt_title: string
  comment_body: string
  comment_prompt_id: number
}

export interface AdminReportHandle {
  status: ReportStatus
  note?: string
  delete_content: boolean
  ban_author: boolean
}

export interface AdminInvite {
  id: number
  code: string
  kind: 'normal' | 'developer'
  creator_name: string
  max_uses: number
  used_count: number
  disabled: boolean
  note: string
  expires_at?: string
  created_at: string
}

export interface AdminInviteInput {
  kind: 'normal' | 'developer'
  count: number
  max_uses: number
  expires_in_days: number
  note?: string
}

export interface AdminInviteUsage {
  id: number
  user_id: number
  username: string
  used_at: string
}

export type RegistrationPolicy = 'open' | 'closed' | 'invite_only'

export interface AdminOperationLog {
  id: number
  operator_id?: number
  operator_name: string
  action: string
  object_type: string
  object_id?: number
  before_data?: unknown
  after_data?: unknown
  note: string
  ip: string
  created_at: string
}

export interface AdminAnnouncement {
  id: number
  title: string
  body: string
  publisher_name: string
  state: AnnouncementState
  published_at?: string
  withdrawn_at?: string
  created_at: string
}

export interface AdminRecycleEntry {
  id: number
  object_type: RecycleObjectType
  object_id: number
  title: string
  owner_id?: number
  owner_name: string
  library_id?: number
  deleted_by?: number
  deleted_by_name: string
  snapshot?: unknown
  deleted_at: string
  expires_at: string
}

export interface AdminConfigValue {
  key: string
  kind: ConfigKind
  description: string
  secret: boolean
  value?: unknown
  effective?: unknown
  source: 'config' | 'env' | ''
  updated_at?: string
}

export interface AdminPurpose {
  id: number
  slug: string
  name_zh: string
  name_en: string
  description_zh: string
  description_en: string
  is_high_risk: boolean
  is_enabled: boolean
  sort_order: number
}

export interface AdminPurposeInput {
  slug: string
  name_zh: string
  name_en: string
  description_zh: string
  description_en: string
  is_high_risk: boolean
  is_enabled: boolean
  sort_order: number
}

export interface AdminCapability {
  id: number
  slug: string
  name_zh: string
  name_en: string
  group_key: 'cognitive' | 'execution' | 'reliability'
  description_zh: string
  description_en: string
  is_enabled: boolean
  sort_order: number
}

export interface AdminCapabilityInput {
  slug: string
  name_zh: string
  name_en: string
  group_key: 'cognitive' | 'execution' | 'reliability'
  description_zh: string
  description_en: string
  is_enabled: boolean
  sort_order: number
}

export interface AdminPromptQuery {
  page?: number
  page_size?: number
  keyword?: string
  visibility?: string
  deleted?: string
  purpose_id?: string
  capability_id?: string
  uploader_id?: string
}

/** registerAdminApi 把查询参数拼成查询串，空值一律省略。 */
function query(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') {
      continue
    }
    search.set(key, String(value))
  }
  const raw = search.toString()
  return raw === '' ? '' : `?${raw}`
}

export const adminApi = {
  // ---------- 模块 1：统计仪表盘 ----------

  dashboard(): Promise<Dashboard> {
    return api.get<Dashboard>('/admin/dashboard')
  },

  // ---------- 模块 2：提示词管理 ----------

  prompts(params: AdminPromptQuery): Promise<Page<AdminPrompt>> {
    return api.get<Page<AdminPrompt>>(
      `/admin/prompts${query({
        page: params.page,
        page_size: params.page_size,
        keyword: params.keyword,
        visibility: params.visibility,
        deleted: params.deleted,
        purpose_id: params.purpose_id,
        capability_id: params.capability_id,
        uploader_id: params.uploader_id,
      })}`,
    )
  },

  prompt(id: number): Promise<AdminPromptDetail> {
    return api.get<AdminPromptDetail>(`/admin/prompts/${id}`)
  },

  deletePrompt(id: number): Promise<void> {
    return api.delete<void>(`/admin/prompts/${id}`)
  },

  restorePrompt(id: number): Promise<void> {
    return api.post<void>(`/admin/prompts/${id}/restore`, undefined)
  },

  purgePrompt(id: number): Promise<void> {
    return api.post<void>(`/admin/prompts/${id}/purge`, undefined)
  },

  markPromptMajor(
    id: number,
    note: string,
  ): Promise<{ version_no: string; archived_ratings: number }> {
    return api.post<{ version_no: string; archived_ratings: number }>(
      `/admin/prompts/${id}/major-mark`,
      { note },
    )
  },

  promptRatings(id: number, page = 1, pageSize = 20): Promise<Page<AdminRating>> {
    return api.get<Page<AdminRating>>(
      `/admin/prompts/${id}/ratings${query({ page, page_size: pageSize })}`,
    )
  },

  promptComments(id: number, page = 1, pageSize = 20): Promise<Page<AdminComment>> {
    return api.get<Page<AdminComment>>(
      `/admin/prompts/${id}/comments${query({ page, page_size: pageSize })}`,
    )
  },

  attachPromptTag(promptId: number, tagId: number): Promise<void> {
    return api.post<void>(`/admin/prompts/${promptId}/tags`, { tag_id: tagId })
  },

  detachPromptTag(promptId: number, tagId: number): Promise<void> {
    return api.delete<void>(`/admin/prompts/${promptId}/tags/${tagId}`)
  },

  recommendTags(
    states: RecommendTagState[],
    kind: string,
    page = 1,
    pageSize = 20,
  ): Promise<Page<AdminRecommendTag>> {
    const search = new URLSearchParams()
    search.set('page', String(page))
    search.set('page_size', String(pageSize))
    for (const state of states) {
      search.append('states', state)
    }
    if (kind !== '') {
      search.set('kind', kind)
    }
    return api.get<Page<AdminRecommendTag>>(`/admin/recommend-tags?${search.toString()}`)
  },

  reviewRecommendTags(
    ids: number[],
    state: 'confirmed' | 'ignored',
    labels?: { label_zh?: string; label_en?: string },
  ): Promise<void> {
    return api.post<void>('/admin/recommend-tags/review', {
      ids,
      state,
      label_zh: labels?.label_zh,
      label_en: labels?.label_en,
    })
  },

  // ---------- 模块 3：模型管理 ----------

  models(params: {
    page?: number
    page_size?: number
    keyword?: string
    include_deleted?: boolean
  }): Promise<Page<AdminModel>> {
    return api.get<Page<AdminModel>>(
      `/admin/models${query({
        page: params.page,
        page_size: params.page_size,
        keyword: params.keyword,
        include_deleted: params.include_deleted === true ? 'true' : undefined,
      })}`,
    )
  },

  createModel(input: AdminModelInput): Promise<AdminModel> {
    return api.post<AdminModel>('/admin/models', input)
  },

  model(id: number): Promise<AdminModel> {
    return api.get<AdminModel>(`/admin/models/${id}`)
  },

  updateModel(id: number, input: AdminModelInput): Promise<AdminModel> {
    return api.patch<AdminModel>(`/admin/models/${id}`, input)
  },

  deleteModel(id: number): Promise<void> {
    return api.delete<void>(`/admin/models/${id}`)
  },

  setModelEnabled(id: number, enabled: boolean): Promise<AdminModel> {
    return api.post<AdminModel>(`/admin/models/${id}/enabled`, { enabled })
  },

  modelPrompts(id: number, page = 1, pageSize = 20): Promise<Page<AdminModelPrompt>> {
    return api.get<Page<AdminModelPrompt>>(
      `/admin/models/${id}/prompts${query({ page, page_size: pageSize })}`,
    )
  },

  modelStats(id: number, page = 1, pageSize = 20): Promise<Page<AdminModelStat>> {
    return api.get<Page<AdminModelStat>>(
      `/admin/models/${id}/stats${query({ page, page_size: pageSize })}`,
    )
  },

  modelSnapshots(id: number, page = 1, pageSize = 20): Promise<Page<AdminModelSnapshot>> {
    return api.get<Page<AdminModelSnapshot>>(
      `/admin/models/${id}/snapshots${query({ page, page_size: pageSize })}`,
    )
  },

  modelImportLogs(action: string, page = 1, pageSize = 20): Promise<Page<AdminModelImportLog>> {
    return api.get<Page<AdminModelImportLog>>(
      `/admin/models/import-logs${query({ action, page, page_size: pageSize })}`,
    )
  },

  /** 预览 models.dev 的导入内容并标出重复项，供二次确认（需求 §13.4）。 */
  previewModelImport(): Promise<{ items: AdminModelImportEntry[]; duplicate_count: number }> {
    return api.post<{ items: AdminModelImportEntry[]; duplicate_count: number }>(
      '/admin/models/import/preview',
      undefined,
    )
  },

  importModels(overwrite: boolean): Promise<AdminModelImportResult> {
    return api.post<AdminModelImportResult>('/admin/models/import', { overwrite })
  },

  // ---------- 模块 3b：分类字典 ----------

  purposes(params: {
    page?: number
    page_size?: number
    keyword?: string
  }): Promise<Page<AdminPurpose>> {
    return api.get<Page<AdminPurpose>>(
      `/admin/purposes${query({
        page: params.page,
        page_size: params.page_size,
        keyword: params.keyword,
      })}`,
    )
  },

  createPurpose(input: AdminPurposeInput): Promise<AdminPurpose> {
    return api.post<AdminPurpose>('/admin/purposes', input)
  },

  updatePurpose(id: number, input: AdminPurposeInput): Promise<AdminPurpose> {
    return api.patch<AdminPurpose>(`/admin/purposes/${id}`, input)
  },

  deletePurpose(id: number): Promise<void> {
    return api.delete<void>(`/admin/purposes/${id}`)
  },

  capabilities(params: {
    page?: number
    page_size?: number
    keyword?: string
  }): Promise<Page<AdminCapability>> {
    return api.get<Page<AdminCapability>>(
      `/admin/capabilities${query({
        page: params.page,
        page_size: params.page_size,
        keyword: params.keyword,
      })}`,
    )
  },

  createCapability(input: AdminCapabilityInput): Promise<AdminCapability> {
    return api.post<AdminCapability>('/admin/capabilities', input)
  },

  updateCapability(id: number, input: AdminCapabilityInput): Promise<AdminCapability> {
    return api.patch<AdminCapability>(`/admin/capabilities/${id}`, input)
  },

  deleteCapability(id: number): Promise<void> {
    return api.delete<void>(`/admin/capabilities/${id}`)
  },

  // ---------- 模块 4：用户管理 ----------

  users(params: {
    page?: number
    page_size?: number
    keyword?: string
    role?: string
    status?: string
  }): Promise<Page<AdminUser>> {
    return api.get<Page<AdminUser>>(
      `/admin/users${query({
        page: params.page,
        page_size: params.page_size,
        keyword: params.keyword,
        role: params.role,
        status: params.status,
      })}`,
    )
  },

  user(id: number): Promise<AdminUser> {
    return api.get<AdminUser>(`/admin/users/${id}`)
  },

  banUser(
    id: number,
    reason: string,
  ): Promise<{ user: AdminUser; transferred_prompt_count: number }> {
    return api.post<{ user: AdminUser; transferred_prompt_count: number }>(
      `/admin/users/${id}/ban`,
      { reason },
    )
  },

  unbanUser(id: number): Promise<AdminUser> {
    return api.post<AdminUser>(`/admin/users/${id}/unban`, undefined)
  },

  setDeveloper(id: number, enabled: boolean): Promise<AdminUser> {
    return api.put<AdminUser>(`/admin/users/${id}/developer`, { enabled })
  },

  userRatings(id: number, page = 1, pageSize = 20): Promise<Page<AdminUserRating>> {
    return api.get<Page<AdminUserRating>>(
      `/admin/users/${id}/ratings${query({ page, page_size: pageSize })}`,
    )
  },

  userComments(id: number, page = 1, pageSize = 20): Promise<Page<AdminUserComment>> {
    return api.get<Page<AdminUserComment>>(
      `/admin/users/${id}/comments${query({ page, page_size: pageSize })}`,
    )
  },

  userLibraries(id: number, page = 1, pageSize = 20): Promise<Page<AdminUserLibrary>> {
    return api.get<Page<AdminUserLibrary>>(
      `/admin/users/${id}/libraries${query({ page, page_size: pageSize })}`,
    )
  },

  userReports(id: number, page = 1, pageSize = 20): Promise<Page<AdminUserReport>> {
    return api.get<Page<AdminUserReport>>(
      `/admin/users/${id}/reports${query({ page, page_size: pageSize })}`,
    )
  },

  // ---------- 模块 5：标签管理 ----------

  tags(params: { page?: number; page_size?: number; keyword?: string }): Promise<Page<AdminTag>> {
    return api.get<Page<AdminTag>>(
      `/admin/tags${query({ page: params.page, page_size: params.page_size, keyword: params.keyword })}`,
    )
  },

  createTag(input: AdminTagInput): Promise<AdminTag> {
    return api.post<AdminTag>('/admin/tags', input)
  },

  updateTag(id: number, input: AdminTagInput): Promise<AdminTag> {
    return api.patch<AdminTag>(`/admin/tags/${id}`, input)
  },

  deleteTag(id: number): Promise<void> {
    return api.delete<void>(`/admin/tags/${id}`)
  },

  // ---------- 模块 6：举报管理 ----------

  reports(params: {
    page?: number
    page_size?: number
    object_type?: string
    status?: string
  }): Promise<Page<AdminReport>> {
    return api.get<Page<AdminReport>>(
      `/admin/reports${query({
        page: params.page,
        page_size: params.page_size,
        object_type: params.object_type,
        status: params.status,
      })}`,
    )
  },

  handleReport(id: number, input: AdminReportHandle): Promise<void> {
    return api.post<void>(`/admin/reports/${id}/handle`, input)
  },

  // ---------- 模块 7：注册与邀请码管理 ----------

  invites(params: {
    page?: number
    page_size?: number
    kind?: string
    disabled?: string
  }): Promise<Page<AdminInvite>> {
    return api.get<Page<AdminInvite>>(
      `/admin/invites${query({
        page: params.page,
        page_size: params.page_size,
        kind: params.kind,
        disabled: params.disabled,
      })}`,
    )
  },

  createInvites(input: AdminInviteInput): Promise<{ items: AdminInvite[] }> {
    return api.post<{ items: AdminInvite[] }>('/admin/invites', input)
  },

  setInviteDisabled(id: number, disabled: boolean): Promise<AdminInvite> {
    return api.post<AdminInvite>(`/admin/invites/${id}/disabled`, { disabled })
  },

  inviteUsages(id: number, page = 1, pageSize = 20): Promise<Page<AdminInviteUsage>> {
    return api.get<Page<AdminInviteUsage>>(
      `/admin/invites/${id}/usages${query({ page, page_size: pageSize })}`,
    )
  },

  registrationPolicy(): Promise<{ policy: RegistrationPolicy }> {
    return api.get<{ policy: RegistrationPolicy }>('/admin/registration-policy')
  },

  // ---------- 模块 8：操作日志 ----------

  operationLogs(params: {
    page?: number
    page_size?: number
    action?: string
    object_type?: string
    operator?: string
    since?: string
    until?: string
  }): Promise<Page<AdminOperationLog>> {
    return api.get<Page<AdminOperationLog>>(
      `/admin/operation-logs${query({
        page: params.page,
        page_size: params.page_size,
        action: params.action,
        object_type: params.object_type,
        operator: params.operator,
        since: params.since,
        until: params.until,
      })}`,
    )
  },

  operationLogActions(): Promise<{ actions: string[]; required: string[] }> {
    return api.get<{ actions: string[]; required: string[] }>('/admin/operation-logs/actions')
  },

  // ---------- 模块 9：公告管理 ----------

  announcements(params: {
    page?: number
    page_size?: number
    state?: string
  }): Promise<Page<AdminAnnouncement>> {
    return api.get<Page<AdminAnnouncement>>(
      `/admin/announcements${query({
        page: params.page,
        page_size: params.page_size,
        state: params.state,
      })}`,
    )
  },

  createAnnouncement(title: string, body: string): Promise<AdminAnnouncement> {
    return api.post<AdminAnnouncement>('/admin/announcements', { title, body })
  },

  updateAnnouncement(id: number, title: string, body: string): Promise<AdminAnnouncement> {
    return api.patch<AdminAnnouncement>(`/admin/announcements/${id}`, { title, body })
  },

  withdrawAnnouncement(id: number): Promise<AdminAnnouncement> {
    return api.post<AdminAnnouncement>(`/admin/announcements/${id}/withdraw`, undefined)
  },

  // ---------- 模块 10：回收站管理 ----------

  recycleBin(params: {
    page?: number
    page_size?: number
    object_type?: string
  }): Promise<Page<AdminRecycleEntry>> {
    return api.get<Page<AdminRecycleEntry>>(
      `/admin/recycle-bin${query({
        page: params.page,
        page_size: params.page_size,
        object_type: params.object_type,
      })}`,
    )
  },

  restoreRecycleEntry(objectType: string, objectID: number): Promise<void> {
    return api.post<void>(`/admin/recycle-bin/${objectType}/${objectID}/restore`, undefined)
  },

  purgeRecycleEntry(objectType: string, objectID: number): Promise<void> {
    return api.post<void>(`/admin/recycle-bin/${objectType}/${objectID}/purge`, undefined)
  },

  // ---------- 模块 11：系统设置 ----------

  settings(): Promise<{ items: AdminConfigValue[]; secret_writable: boolean }> {
    return api.get<{ items: AdminConfigValue[]; secret_writable: boolean }>('/admin/settings')
  },

  saveSetting(key: string, value: unknown): Promise<void> {
    return api.put<void>(`/admin/settings/${key}`, { value })
  },

  sendTestEmail(to: string): Promise<void> {
    return api.post<void>('/admin/settings/smtp-test', { to })
  },
}
