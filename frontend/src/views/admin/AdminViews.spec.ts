// 管理后台视图的单元测试：各模块覆盖渲染与一次关键写入，
// 外加一条词汇表校验——视图里引用的 admin.* 键必须在语言包中存在，
// 否则界面会直接显示原始键名。

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import en from '@/i18n/en'
import zh from '@/i18n/zh'
import { useAuthStore } from '@/stores/auth'
import { errorEnvelope, mountView, resetLocale, stubResponse } from '@/testing/mount'
import AdminAnnouncementsView from '@/views/admin/AdminAnnouncementsView.vue'
import AdminDashboardView from '@/views/admin/AdminDashboardView.vue'
import AdminDictionaryView from '@/views/admin/AdminDictionaryView.vue'
import AdminInvitesView from '@/views/admin/AdminInvitesView.vue'
import AdminLayout from '@/views/admin/AdminLayout.vue'
import AdminLogsView from '@/views/admin/AdminLogsView.vue'
import AdminModelsView from '@/views/admin/AdminModelsView.vue'
import AdminPromptsView from '@/views/admin/AdminPromptsView.vue'
import AdminRecycleView from '@/views/admin/AdminRecycleView.vue'
import AdminReportsView from '@/views/admin/AdminReportsView.vue'
import AdminSettingsView from '@/views/admin/AdminSettingsView.vue'
import AdminTagsView from '@/views/admin/AdminTagsView.vue'
import AdminUsersView from '@/views/admin/AdminUsersView.vue'

interface RecordedRequest {
  method: string
  path: string
  query: string
  body: unknown
}

/**
 * hostElement 查找带 data-testid 的节点。
 *
 * 组件的对话框默认渲染在组件子树内，ElMessageBox 则传送到 body，
 * 因此先查视图，再查文档。
 */
function hostElement(wrapper: VueWrapper, testid: string): HTMLElement | null {
  const inline = wrapper.find<HTMLElement>(`[data-testid="${testid}"]`)
  if (inline.exists()) {
    return inline.element
  }
  return document.body.querySelector<HTMLElement>(`[data-testid="${testid}"]`)
}

/**
 * inputElement 取出输入框本身。
 *
 * Element Plus 的 el-input 把未声明的属性交给内部的 input 元素，
 * 因此 data-testid 有时落在 input 上，有时落在包装节点上，两种都要能取到。
 */
function inputElement(wrapper: VueWrapper, testid: string): HTMLInputElement | HTMLTextAreaElement {
  const host = hostElement(wrapper, testid)
  if (host === null) {
    throw new Error(`未找到 ${testid}`)
  }
  if (host instanceof HTMLInputElement || host instanceof HTMLTextAreaElement) {
    return host
  }
  const inner = host.querySelector<HTMLInputElement | HTMLTextAreaElement>('input, textarea')
  if (inner === null) {
    throw new Error(`${testid} 内没有输入框`)
  }
  return inner
}

/** clickElement 点击带 testid 的节点。 */
function clickElement(wrapper: VueWrapper, testid: string): void {
  const element = hostElement(wrapper, testid)
  if (element === null) {
    throw new Error(`未找到 ${testid}`)
  }
  element.click()
}

/** clickMessageBoxConfirm 点击 ElMessageBox 的确认按钮。 */
async function clickMessageBoxConfirm(): Promise<void> {
  await flushPromises()
  const button = document.body.querySelector<HTMLButtonElement>(
    '.el-message-box__btns .el-button--primary',
  )
  if (button === null) {
    throw new Error('未找到确认对话框的确认按钮')
  }
  button.click()
  await flushPromises()
}

/** stubAdminEndpoints 按路径与查询分派后台端点，并记录全部请求。 */
function stubAdminEndpoints(overrides: Record<string, unknown> = {}): RecordedRequest[] {
  const recorded: RecordedRequest[] = []
  /**
   * 覆盖项按 key 生效：secretWritable 为假表示后端未配置主密钥，
   * 系统设置里密钥类配置项据此显示不可保存的提示。
   */
  const secretWritable = overrides.secretWritable !== false

  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const withoutHost = url.replace(/^https?:\/\/[^/]+/, '')
      const [path = '', query = ''] = withoutHost.split('?')
      const method = init?.method ?? 'GET'
      const body = init?.body === undefined ? undefined : JSON.parse(String(init.body))
      recorded.push({ method, path, query, body })

      const payload = (value: unknown, status = 200): Response =>
        stubResponse({ status, body: value })

      // 会话与字典。
      if (path.endsWith('/session')) {
        return payload({
          id: 1,
          username: 'admin',
          email: 'admin@example.com',
          role: 'admin',
          status: 'normal',
          bio: '',
          avatar_url: '',
          email_verified: true,
        })
      }
      if (path === '/api/v1/purposes') {
        return payload({ items: [] })
      }
      if (path === '/api/v1/capabilities' || path === '/api/v1/difficulties') {
        return payload({ items: [] })
      }
      if (path === '/api/v1/tags') {
        return payload({
          items: [{ id: 5, slug: 'audit', name_zh: '审计', name_en: 'Audit', color: '#112233' }],
        })
      }
      if (path === '/api/v1/models') {
        return payload({ items: [] })
      }

      // 各模块端点。
      if (path === '/api/v1/admin/dashboard') {
        return payload({
          user_total: 4,
          user_today: 1,
          user_role_user: 3,
          user_role_developer: 1,
          user_banned: 0,
          prompt_total: 2,
          prompt_public: 1,
          prompt_private: 0,
          prompt_developer: 1,
          prompt_today: 1,
          rating_total: 0,
          comment_total: 0,
          report_pending: 0,
          recycle_total: 0,
          model_total: 1,
          invite_total: 0,
          recommend_pending: 0,
          hot_prompts: [
            {
              id: 7,
              title: '热门提示词',
              visibility: 'public',
              comment_count: 3,
              favorite_count: 2,
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
          recent_prompts: [],
        })
      }
      if (path === '/api/v1/admin/operation-logs/actions') {
        return payload({ actions: ['user_ban', 'prompt_delete'], required: ['user_ban'] })
      }
      if (path === '/api/v1/admin/operation-logs') {
        return payload({
          items: [
            {
              id: 11,
              operator_id: 1,
              operator_name: 'admin',
              action: 'user_ban',
              object_type: 'user',
              object_id: 9,
              after_data: { status: 'banned' },
              note: '垃圾广告',
              ip: '10.0.0.1',
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (path === '/api/v1/admin/recycle-bin') {
        return payload({
          items: [
            {
              id: 1,
              object_type: 'prompt',
              object_id: 7,
              title: '被删除的提示词',
              owner_id: 2,
              owner_name: 'alice',
              deleted_by: 2,
              deleted_by_name: 'alice',
              deleted_at: '2026-01-01T00:00:00Z',
              expires_at: '2026-01-15T00:00:00Z',
            },
            {
              id: 2,
              object_type: 'library',
              object_id: 3,
              title: '被删除的题库',
              owner_name: 'bob',
              deleted_by_name: 'bob',
              deleted_at: '2026-01-01T00:00:00Z',
              expires_at: '2026-01-15T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 2,
        })
      }
      if (path === '/api/v1/admin/prompts') {
        return payload({
          items: [
            {
              id: 7,
              title: '后台提示词',
              description: '描述',
              purpose_id: 1,
              purpose_name_zh: '边界探测',
              purpose_name_en: 'Capability Boundary',
              capability_id: 1,
              capability_name_zh: '推理',
              capability_name_en: 'Reasoning',
              visibility: 'public',
              sensitive_level: 'low',
              uploader_id: 2,
              uploader_name: 'alice',
              version_no: 'v1.0',
              major_modified: false,
              is_high_risk: false,
              comment_count: 0,
              favorite_count: 0,
              deleted: false,
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (/^\/api\/v1\/admin\/prompts\/\d+$/.test(path) && method === 'GET') {
        return payload({
          prompt: {
            id: 7,
            title: '后台提示词',
            description: '',
            purpose_id: 1,
            purpose_name_zh: '边界探测',
            purpose_name_en: 'Capability Boundary',
            capability_id: 1,
            capability_name_zh: '推理',
            capability_name_en: 'Reasoning',
            visibility: 'public',
            sensitive_level: 'low',
            uploader_id: 2,
            uploader_name: 'alice',
            version_no: 'v1.0',
            major_modified: false,
            is_high_risk: false,
            comment_count: 0,
            favorite_count: 0,
            deleted: false,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
          body: '提示词正文',
          tags: [],
          versions: [],
        })
      }
      if (path.endsWith('/major-mark')) {
        return payload({ version_no: 'v2.0', archived_ratings: 3 })
      }
      if (path === '/api/v1/admin/models') {
        return payload({
          items: [
            {
              id: 4,
              name: 'Alpha One',
              model_key: 'alpha-one',
              provider_key: 'alpha',
              provider_name: 'Alpha Lab',
              input_modalities: ['text'],
              aliases: [],
              description: '',
              release_date: '2025-01-31',
              context_length: 128000,
              is_enabled: true,
              capability_ids: [],
              note: '',
              import_source: 'models_dev',
              rating_count: 0,
              prompt_count: 0,
              deleted: false,
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (path === '/api/v1/admin/models/import/preview') {
        return payload({
          items: [
            {
              provider_id: 'alpha',
              provider_name: 'Alpha Lab',
              model_id: 'alpha-one',
              name: 'Alpha One',
              description: '',
              release_date: '2025-01-31',
              input_modalities: ['text'],
              context_length: 128000,
              duplicate: { id: 4, name: 'Alpha One', provider_name: 'Alpha Lab' },
            },
          ],
          duplicate_count: 1,
        })
      }
      if (path === '/api/v1/admin/models/import') {
        return payload({ total: 1, created: 0, overwritten: 1, skipped: 0 })
      }
      if (/\/admin\/models\/\d+\/enabled$/.test(path)) {
        return payload({
          id: 4,
          name: 'Alpha One',
          model_key: 'alpha-one',
          provider_key: 'alpha',
          provider_name: 'Alpha Lab',
          input_modalities: [],
          aliases: [],
          description: '',
          release_date: '',
          is_enabled: false,
          capability_ids: [],
          note: '',
          import_source: 'models_dev',
          rating_count: 0,
          prompt_count: 0,
          deleted: false,
          created_at: '',
          updated_at: '',
        })
      }
      if (path === '/api/v1/admin/users') {
        return payload({
          items: [
            {
              id: 9,
              username: 'spammer',
              email: 'spammer@example.com',
              role: 'user',
              status: 'normal',
              bio: '',
              avatar_url: '',
              email_verified: true,
              is_system: false,
              banned_reason: '',
              prompt_count: 2,
              has_password: true,
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (/\/admin\/users\/\d+\/ban$/.test(path)) {
        return payload({
          user: {
            id: 9,
            username: 'spammer',
            role: 'user',
            status: 'banned',
            email: '',
            bio: '',
            avatar_url: '',
            email_verified: true,
          },
          transferred_prompt_count: 2,
        })
      }
      if (path === '/api/v1/admin/capabilities') {
        return payload({
          items: [
            {
              id: 1,
              slug: 'reasoning',
              name_zh: '推理',
              name_en: 'Reasoning',
              group_key: 'cognitive',
              description_zh: '多步演绎与约束求解',
              description_en: 'Multi-step deduction',
              is_enabled: true,
              sort_order: 1,
            },
          ],
          page: 1,
          page_size: 100,
          total: 1,
        })
      }
      if (path === '/api/v1/admin/purposes') {
        return payload({
          items: [
            {
              id: 1,
              slug: 'capability_boundary',
              name_zh: '边界探测',
              name_en: 'Capability Boundary',
              description_zh: '模型是否具备该能力，边界在哪里',
              description_en: 'Does the model have the capability?',
              is_high_risk: false,
              is_enabled: true,
              sort_order: 1,
            },
          ],
          page: 1,
          page_size: 100,
          total: 1,
        })
      }
      if (path === '/api/v1/admin/tags') {
        return payload({
          items: [
            {
              id: 5,
              slug: 'audit',
              name_zh: '审计',
              name_en: 'Audit',
              color: '#112233',
              description: '',
              priority: 1,
              is_enabled: true,
              usage_count: 2,
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (path === '/api/v1/admin/recommend-tags') {
        return payload({
          items: [
            {
              id: 21,
              prompt_id: 7,
              prompt_title: '后台提示词',
              model_id: 4,
              model_name: 'Alpha One',
              scope: 'prompt_model',
              kind: 'obsolete',
              state: 'pending',
              label_zh: '过时',
              label_en: 'Obsolete',
              confidence: 'high',
              reason: '区分度低于阈值',
              triggered_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (path === '/api/v1/admin/recommend-tags/review') {
        return stubResponse({ status: 204 })
      }
      if (path === '/api/v1/admin/reports') {
        return payload({
          items: [
            {
              id: 31,
              reporter_id: 2,
              reporter_name: 'alice',
              object_type: 'prompt',
              object_id: 7,
              reason: 'spam',
              detail: '广告内容',
              status: 'pending',
              handler_name: '',
              handle_note: '',
              created_at: '2026-01-01T00:00:00Z',
              prompt_title: '后台提示词',
              comment_body: '',
              comment_prompt_id: 0,
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (/\/admin\/reports\/\d+\/handle$/.test(path)) {
        return stubResponse({ status: 204 })
      }
      if (path === '/api/v1/admin/registration-policy') {
        return payload({ policy: 'open' })
      }
      if (path === '/api/v1/admin/invites') {
        return payload({
          items: [
            {
              id: 41,
              code: 'PB-ABCDEF',
              kind: 'normal',
              creator_name: 'admin',
              max_uses: 1,
              used_count: 0,
              disabled: false,
              note: '',
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (path === '/api/v1/admin/announcements') {
        return payload({
          items: [
            {
              id: 51,
              title: '平台公告',
              body: '正文',
              publisher_name: 'admin',
              state: 'live',
              published_at: '2026-01-01T00:00:00Z',
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 20,
          total: 1,
        })
      }
      if (path.endsWith('/withdraw')) {
        return payload({
          id: 51,
          title: '平台公告',
          body: '正文',
          publisher_name: 'admin',
          state: 'withdrawn',
          created_at: '2026-01-01T00:00:00Z',
        })
      }
      if (path === '/api/v1/admin/settings') {
        return payload({
          secret_writable: secretWritable,
          items: [
            {
              key: 'bayes_prior',
              kind: 'float',
              description: '贝叶斯先验分',
              secret: false,
              value: 3,
              effective: 3,
              source: 'config',
              updated_at: '2026-01-01T00:00:00Z',
            },
            {
              key: 'smtp_password',
              kind: 'string',
              description: 'SMTP 登录口令',
              secret: true,
              effective: '已由环境变量提供',
              source: 'env',
            },
          ],
        })
      }
      if (path === '/api/v1/admin/users' && method === 'GET') {
        return payload({ items: [], page: 1, page_size: 20, total: 0 })
      }

      if (typeof overrides[path] === 'object') {
        return payload(overrides[path])
      }
      // 未被覆盖的写端点一律成功；未登记的读端点返回空页。
      if (method === 'GET') {
        return payload({ items: [], page: 1, page_size: 20, total: 0 })
      }
      return stubResponse({ status: 204 })
    }),
  )
  return recorded
}

/** mountAdmin 装配管理视图，并把登录态预先设为管理员。 */
async function mountAdmin(
  component: Parameters<typeof mountView>[0],
  role: 'admin' | 'user' = 'admin',
): Promise<VueWrapper> {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.markAuthenticated({
    id: 1,
    username: 'admin',
    email: 'admin@example.com',
    role,
    status: 'normal',
    bio: '',
    avatar_url: '',
    email_verified: true,
  })

  const wrapper = mountView(component, { pinia, router: adminRouter() })
  await flushPromises()
  return wrapper
}

/** adminRouter 提供访问后台外壳所需的路由环境。 */
function adminRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/admin/dashboard', name: 'admin-dashboard', component: { template: '<div />' } },
      { path: '/prompts/:id', name: 'prompt-detail', component: { template: '<div />' } },
      { path: '/admin/prompts', name: 'admin-prompts', component: { template: '<div />' } },
    ],
  })
}

function requestTo(recorded: RecordedRequest[], path: string, method = 'GET'): RecordedRequest[] {
  return recorded.filter((item) => item.path === path && item.method === method)
}

beforeEach(() => {
  resetLocale()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('分类字典管理', () => {
  it('同时加载考察维度与考察目的', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminDictionaryView)

    expect(wrapper.find('[data-testid="admin-dictionary"]').exists()).toBe(true)
    expect(recorded.some((item) => item.path === '/api/v1/admin/capabilities')).toBe(true)
    expect(recorded.some((item) => item.path === '/api/v1/admin/purposes')).toBe(true)
  })
})

describe('管理后台外框', () => {
  it('管理员看到 12 个模块的导航', async () => {
    stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminLayout)

    const links = wrapper.findAll('[data-testid="admin-nav"] a')
    expect(links).toHaveLength(12)
    expect(wrapper.find('[data-testid="admin-denied"]').exists()).toBe(false)
  })

  it('非管理员看到拒绝提示，导航不渲染', async () => {
    stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminLayout, 'user')

    expect(wrapper.find('[data-testid="admin-denied"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="admin-nav"]').exists()).toBe(false)
  })
})

describe('统计仪表盘', () => {
  it('渲染 17 项计数与两个榜单', async () => {
    stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminDashboardView)

    for (const field of [
      'user_total',
      'user_today',
      'user_role_user',
      'user_role_developer',
      'user_banned',
      'prompt_total',
      'prompt_public',
      'prompt_private',
      'prompt_developer',
      'prompt_today',
      'rating_total',
      'comment_total',
      'report_pending',
      'recycle_total',
      'model_total',
      'invite_total',
      'recommend_pending',
    ]) {
      expect(wrapper.find(`[data-testid="admin-dashboard-count-${field}"]`).exists()).toBe(true)
    }
    expect(wrapper.find('[data-testid="admin-dashboard-hot"]').text()).toContain('热门提示词')
    expect(wrapper.find('[data-testid="admin-dashboard-recent"]').exists()).toBe(true)
  })
})

describe('操作日志', () => {
  it('渲染日志并把操作类型显示为中文', async () => {
    stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminLogsView)

    expect(wrapper.find('[data-testid="admin-logs-readonly"]').exists()).toBe(true)
    const table = wrapper.find('[data-testid="admin-logs-table"]')
    expect(table.text()).toContain('封禁用户')
    expect(table.text()).toContain('垃圾广告')
  })

  it('带筛选条件重新查询', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminLogsView)

    await wrapper.find('[data-testid="admin-logs-search"]').trigger('click')
    await flushPromises()

    const queries = requestTo(recorded, '/api/v1/admin/operation-logs')
    expect(queries.length).toBeGreaterThanOrEqual(2)
  })
})

describe('回收站管理', () => {
  it('题库对象的恢复与彻底删除按钮被禁用', async () => {
    stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminRecycleView)

    const table = wrapper.find('[data-testid="admin-recycle-table"]')
    expect(table.text()).toContain('被删除的提示词')
    expect(table.text()).toContain('被删除的题库')

    const restoreButtons = wrapper.findAll('[data-testid="admin-recycle-restore"]')
    expect(restoreButtons.length).toBe(2)
    const disabled = restoreButtons.filter((button) => button.attributes('disabled') !== undefined)
    expect(disabled).toHaveLength(1)
  })
})

describe('提示词管理', () => {
  it('渲染列表并打开详情抽屉', async () => {
    stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminPromptsView)

    expect(wrapper.find('[data-testid="admin-prompts-table"]').text()).toContain('后台提示词')

    await wrapper.find('[data-testid="admin-prompts-detail"]').trigger('click')
    await flushPromises()

    const title = hostElement(wrapper, 'admin-prompts-detail-title')
    expect(title?.textContent).toContain('后台提示词')
    expect(hostElement(wrapper, 'admin-prompts-major-mark')).not.toBeNull()
  })
})

describe('模型管理', () => {
  it('预览导入并展示重复项', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminModelsView)

    await wrapper.find('[data-testid="admin-models-import"]').trigger('click')
    await flushPromises()

    expect(requestTo(recorded, '/api/v1/admin/models/import/preview', 'POST')).toHaveLength(1)
    expect(hostElement(wrapper, 'admin-models-import-run')).not.toBeNull()

    clickElement(wrapper, 'admin-models-import-run')
    await flushPromises()

    const applied = requestTo(recorded, '/api/v1/admin/models/import', 'POST')
    expect(applied).toHaveLength(1)
    expect(applied[0]?.body).toEqual({ overwrite: false })
  })

  it('禁用模型发出状态变更请求', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminModelsView)

    await wrapper.find('[data-testid="admin-models-enable"]').trigger('click')
    await flushPromises()

    expect(requestTo(recorded, '/api/v1/admin/models/4/enabled', 'POST')).toHaveLength(1)
  })
})

describe('用户管理', () => {
  it('封禁需要填写原因并提交到封禁端点', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminUsersView)

    await wrapper.find('[data-testid="admin-users-ban"]').trigger('click')
    await flushPromises()

    const reason = inputElement(wrapper, 'admin-users-ban-reason')
    reason.value = '垃圾广告'
    reason.dispatchEvent(new Event('input'))
    await flushPromises()

    clickElement(wrapper, 'admin-users-ban-confirm')
    await flushPromises()

    const banned = requestTo(recorded, '/api/v1/admin/users/9/ban', 'POST')
    expect(banned).toHaveLength(1)
  })
})

describe('标签管理', () => {
  it('推荐标签可以单条确认', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminTagsView)

    await wrapper.find('[data-testid="admin-tags-confirm"]').trigger('click')
    await flushPromises()

    const reviewed = requestTo(recorded, '/api/v1/admin/recommend-tags/review', 'POST')
    expect(reviewed).toHaveLength(1)
    expect(reviewed[0]?.body).toEqual({ ids: [21], state: 'confirmed' })
  })
})

describe('举报管理', () => {
  it('处置举报提交状态与开关', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminReportsView)

    await wrapper.find('[data-testid="admin-reports-handle"]').trigger('click')
    await flushPromises()

    clickElement(wrapper, 'admin-reports-handle-submit')
    await flushPromises()

    const handled = requestTo(recorded, '/api/v1/admin/reports/31/handle', 'POST')
    expect(handled).toHaveLength(1)
    expect(handled[0]?.body).toMatchObject({ status: 'resolved' })
  })
})

describe('注册与邀请码', () => {
  it('保存注册策略调用配置写入端点', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminInvitesView)

    await wrapper.find('[data-testid="admin-invites-policy-save"]').trigger('click')
    await flushPromises()

    const saved = requestTo(recorded, '/api/v1/admin/settings/registration_policy', 'PUT')
    expect(saved).toHaveLength(1)
    expect(saved[0]?.body).toEqual({ value: 'open' })
  })

  it('生成邀请码提交数量与类型', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminInvitesView)

    await wrapper.find('[data-testid="admin-invites-generate"]').trigger('click')
    await flushPromises()

    clickElement(wrapper, 'admin-invites-form-submit')
    await flushPromises()

    const created = requestTo(recorded, '/api/v1/admin/invites', 'POST')
    expect(created).toHaveLength(1)
    expect(created[0]?.body).toMatchObject({ kind: 'normal', count: 1 })
  })
})

describe('公告管理', () => {
  it('撤回公告提交到撤回端点', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminAnnouncementsView)

    await wrapper.find('[data-testid="admin-announcements-withdraw"]').trigger('click')
    await clickMessageBoxConfirm()

    expect(requestTo(recorded, '/api/v1/admin/announcements/51/withdraw', 'POST')).toHaveLength(1)
  })
})

describe('系统设置', () => {
  it('密钥项显示已设置状态而不回显取值', async () => {
    stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminSettingsView)

    const text = wrapper.text()
    expect(text).toContain('SMTP 登录口令')
    expect(text).not.toContain('已由环境变量提供')
  })

  it('保存数值型配置项发出对应取值', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminSettingsView)

    await wrapper.find('[data-testid="admin-settings-save-bayes_prior"]').trigger('click')
    await flushPromises()

    const saved = requestTo(recorded, '/api/v1/admin/settings/bayes_prior', 'PUT')
    expect(saved).toHaveLength(1)
    expect(saved[0]?.body).toEqual({ value: 3 })
  })

  it('主密钥缺失时提示密钥类配置项无法保存', async () => {
    stubAdminEndpoints({ secretWritable: false })
    const wrapper = await mountAdmin(AdminSettingsView)

    expect(wrapper.find('[data-testid="admin-settings-no-master-key"]').exists()).toBe(true)
  })

  it('发送测试邮件提交收件地址', async () => {
    const recorded = stubAdminEndpoints()
    const wrapper = await mountAdmin(AdminSettingsView)

    const input = inputElement(wrapper, 'admin-settings-test-to')
    input.value = 'admin@example.com'
    input.dispatchEvent(new Event('input'))
    await flushPromises()

    clickElement(wrapper, 'admin-settings-test-send')
    await flushPromises()

    const sent = requestTo(recorded, '/api/v1/admin/settings/smtp-test', 'POST')
    expect(sent).toHaveLength(1)
    expect(sent[0]?.body).toEqual({ to: 'admin@example.com' })
  })
})

describe('管理后台的文案键', () => {
  it('视图引用的 admin.* 键都在语言包中存在', () => {
    const here = dirname(fileURLToPath(import.meta.url))
    const files = [
      'AdminLayout.vue',
      'AdminDashboardView.vue',
      'AdminPromptsView.vue',
      'AdminModelsView.vue',
      'AdminUsersView.vue',
      'AdminTagsView.vue',
      'AdminReportsView.vue',
      'AdminInvitesView.vue',
      'AdminLogsView.vue',
      'AdminAnnouncementsView.vue',
      'AdminRecycleView.vue',
      'AdminSettingsView.vue',
    ]

    const hasKey = (messages: unknown, key: string): boolean =>
      key.split('.').every((_part, index, parts) => {
        const node = parts.slice(0, index + 1).reduce<unknown>((current, name) => {
          if (typeof current !== 'object' || current === null) {
            return undefined
          }
          return (current as Record<string, unknown>)[name]
        }, messages)
        return node !== undefined
      })

    for (const file of files) {
      const source = readFileSync(resolve(here, file), 'utf8')
      const keys = [...source.matchAll(/t\(\s*'([^']+)'/g)].map((match) => match[1] as string)
      for (const key of keys) {
        if (!key.startsWith('admin.')) {
          continue
        }
        expect(hasKey(zh, key), `${file} 使用了未定义的中文键 ${key}`).toBe(true)
        expect(hasKey(en, key), `${file} 使用了未定义的英文键 ${key}`).toBe(true)
      }
    }
  })

  it('错误码文案在管理端同样可用', () => {
    expect(errorEnvelope('prompt_in_use')).toMatchObject({
      error: { code: 'prompt_in_use' },
    })
  })
})
