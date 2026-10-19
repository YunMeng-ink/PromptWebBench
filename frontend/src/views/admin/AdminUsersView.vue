// 用户管理（需求 §17.4）。
// 列表提供关键词、身份组与账户状态筛选；账户详情按四个页签分开展示，
// 封禁、解封与开发者身份都会改动账户数据和操作日志，因此每次改动后重新拉取列表。

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  type AdminUser,
  type AdminUserComment,
  type AdminUserLibrary,
  type AdminUserRating,
  type AdminUserReport,
  adminApi,
} from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/datetime'

const { t, locale } = useI18n()
const { messageOf } = useErrorMessage()
const router = useRouter()
const auth = useAuthStore()

/** DetailTab 是账户详情对话框里的页签取值。 */
type DetailTab = 'ratings' | 'comments' | 'libraries' | 'reports'

const rows = ref<AdminUser[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const failure = ref('')

const filters = ref({ keyword: '', role: '', status: '' })

const selected = ref<AdminUser | null>(null)
const detailTab = ref<DetailTab>('ratings')
const detailLoading = ref(false)
const ratings = ref<AdminUserRating[]>([])
const comments = ref<AdminUserComment[]>([])
const libraries = ref<AdminUserLibrary[]>([])
const reports = ref<AdminUserReport[]>([])

const banTarget = ref<AdminUser | null>(null)
const banReason = ref('')
const busy = ref(false)

/** currentUserId 用来判断某一行是否是当前登录的管理员自己。 */
const currentUserId = computed(() => auth.user?.id ?? 0)

/** roleText 把身份组取值映射为文案；未登记的取值原样显示。 */
function roleText(role: string): string {
  const labels: Record<string, string> = {
    user: t('role.user'),
    developer: t('role.developer'),
    admin: t('role.admin'),
  }
  return labels[role] ?? role
}

/** statusText 把账户状态映射为文案；未登记的取值原样显示。 */
function statusText(status: string): string {
  const labels: Record<string, string> = {
    normal: t('status.normal'),
    unverified: t('status.unverified'),
    banned: t('status.banned'),
  }
  return labels[status] ?? status
}

/** statusTagType 给三种账户状态分配颜色，未登记的取值按信息色处理。 */
function statusTagType(status: string): 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'normal':
      return 'success'
    case 'unverified':
      return 'warning'
    case 'banned':
      return 'danger'
    default:
      return 'info'
  }
}

/** objectTypeText 把举报对象映射为文案；未登记的取值原样显示。 */
function objectTypeText(objectType: string): string {
  const labels: Record<string, string> = {
    prompt: t('admin.reports.typePrompt'),
    comment: t('admin.reports.typeComment'),
  }
  return labels[objectType] ?? objectType
}

/** reportStatusText 把举报处置状态映射为文案；未登记的取值原样显示。 */
function reportStatusText(status: string): string {
  const labels: Record<string, string> = {
    pending: t('admin.reports.statusPending'),
    ignored: t('admin.reports.statusIgnored'),
    resolved: t('admin.reports.statusResolved'),
  }
  return labels[status] ?? status
}

/** reasonText 把举报原因映射为文案；未登记的取值原样显示。 */
function reasonText(reason: string): string {
  const labels: Record<string, string> = {
    bad_content: t('prompt.reportReasonBadContent'),
    illegal: t('prompt.reportReasonIllegal'),
    porn: t('prompt.reportReasonPorn'),
    attack: t('prompt.reportReasonAttack'),
    privacy: t('prompt.reportReasonPrivacy'),
    spam: t('prompt.reportReasonSpam'),
    dangerous: t('prompt.reportReasonDangerous'),
    other: t('prompt.reportReasonOther'),
  }
  return labels[reason] ?? reason
}

/** formatTime 把接口返回的时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  return formatDateTime(raw, locale.value)
}

/** canModerate 判断这一行是否允许封禁、解封：系统账户与管理员自己都不可操作。 */
function canModerate(row: AdminUser): boolean {
  return !row.is_system && row.id !== currentUserId.value
}

async function loadUsers(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.users({
      page: page.value,
      page_size: pageSize,
      keyword: filters.value.keyword,
      role: filters.value.role,
      status: filters.value.status,
    })
    rows.value = result.items
    total.value = result.total
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

/** loadDetailTab 拉取当前页签的数据；详情对话框每次切换页签都重新读取。 */
async function loadDetailTab(): Promise<void> {
  const user = selected.value
  if (user === null) {
    return
  }
  detailLoading.value = true
  try {
    switch (detailTab.value) {
      case 'ratings': {
        ratings.value = (await adminApi.userRatings(user.id)).items
        break
      }
      case 'comments': {
        comments.value = (await adminApi.userComments(user.id)).items
        break
      }
      case 'libraries': {
        libraries.value = (await adminApi.userLibraries(user.id)).items
        break
      }
      case 'reports': {
        reports.value = (await adminApi.userReports(user.id)).items
        break
      }
    }
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    detailLoading.value = false
  }
}

function search(): void {
  page.value = 1
  void loadUsers()
}

function resetFilters(): void {
  filters.value = { keyword: '', role: '', status: '' }
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadUsers()
}

function openDetail(row: AdminUser): void {
  // 切换到另一个账户时清空上一个账户的四个页签数据，避免内容串号。
  selected.value = row
  detailTab.value = 'ratings'
  ratings.value = []
  comments.value = []
  libraries.value = []
  reports.value = []
  void loadDetailTab()
}

function closeDetail(): void {
  selected.value = null
}

function openBan(row: AdminUser): void {
  banTarget.value = row
  banReason.value = ''
}

function closeBan(): void {
  banTarget.value = null
  banReason.value = ''
}

/** submitBan 提交封禁；原因随请求发给后端并写入操作日志。 */
async function submitBan(): Promise<void> {
  const target = banTarget.value
  if (target === null) {
    return
  }
  busy.value = true
  try {
    const result = await adminApi.banUser(target.id, banReason.value)
    ElMessage.success(t('admin.users.banDone', { count: result.transferred_prompt_count }))
    closeBan()
    await loadUsers()
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    busy.value = false
  }
}

async function unban(row: AdminUser): Promise<void> {
  busy.value = true
  try {
    await adminApi.unbanUser(row.id)
    ElMessage.success(t('admin.users.unbanDone'))
    await loadUsers()
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    busy.value = false
  }
}

/** toggleDeveloper 按当前身份组取反：普通用户授予开发者身份，开发者撤销。 */
async function toggleDeveloper(row: AdminUser): Promise<void> {
  const granted = row.role !== 'developer'
  busy.value = true
  try {
    await adminApi.setDeveloper(row.id, granted)
    ElMessage.success(
      granted ? t('admin.users.developerGranted') : t('admin.users.developerRevoked'),
    )
    await loadUsers()
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    busy.value = false
  }
}

/** viewUploads 带着上传者筛选跳转到提示词管理。 */
function viewUploads(row: AdminUser): void {
  void router.push({ path: '/admin/prompts', query: { uploader_id: String(row.id) } })
}

onMounted(async () => {
  await loadUsers()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title" data-testid="admin-users-title">
        {{ t('admin.users.title') }}
      </h2>
    </header>

    <div class="pb-admin-module__filters">
      <el-input
        v-model="filters.keyword"
        class="pb-admin-module__filter"
        :placeholder="t('admin.users.keyword')"
        clearable
        data-testid="admin-users-keyword"
        @keyup.enter="search"
      />
      <el-select
        v-model="filters.role"
        class="pb-admin-module__filter"
        :placeholder="t('admin.users.role')"
        data-testid="admin-users-role"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('role.user')" value="user" />
        <el-option :label="t('role.developer')" value="developer" />
        <el-option :label="t('role.admin')" value="admin" />
      </el-select>
      <el-select
        v-model="filters.status"
        class="pb-admin-module__filter"
        :placeholder="t('admin.users.status')"
        data-testid="admin-users-status"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('status.normal')" value="normal" />
        <el-option :label="t('status.unverified')" value="unverified" />
        <el-option :label="t('status.banned')" value="banned" />
      </el-select>
      <el-button type="primary" size="small" data-testid="admin-users-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-users-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-users-error">{{ failure }}</p>
    <p
      v-if="!loading && rows.length === 0"
      class="pb-admin-module__empty"
      data-testid="admin-users-empty"
    >
      {{ t('admin.common.empty') }}
    </p>

    <el-table v-if="rows.length > 0" :data="rows" size="small" data-testid="admin-users-table">
      <el-table-column prop="id" :label="t('admin.common.id')" width="70" />
      <el-table-column prop="username" :label="t('me.username')" width="140" />
      <el-table-column prop="email" :label="t('admin.users.address')" min-width="200" />
      <el-table-column :label="t('admin.users.role')" width="110">
        <template #default="{ row }: { row: AdminUser }">{{ roleText(row.role) }}</template>
      </el-table-column>
      <el-table-column :label="t('admin.users.status')" width="100">
        <template #default="{ row }: { row: AdminUser }">
          <el-tag :type="statusTagType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="prompt_count" :label="t('admin.users.promptCount')" width="120" />
      <el-table-column :label="t('admin.users.systemAccount')" width="110">
        <template #default="{ row }: { row: AdminUser }">
          {{ row.is_system ? t('admin.users.systemAccount') : t('admin.common.none') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.users.hasPassword')" width="130">
        <template #default="{ row }: { row: AdminUser }">
          {{ row.has_password ? t('admin.users.hasPassword') : t('admin.users.noPassword') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.users.registeredAt')" width="180">
        <template #default="{ row }: { row: AdminUser }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('admin.common.actions')" min-width="320">
        <template #default="{ row }: { row: AdminUser }">
          <el-button size="small" text data-testid="admin-users-detail" @click="openDetail(row)">
            {{ t('admin.common.detail') }}
          </el-button>
          <el-button
            size="small"
            text
            data-testid="admin-users-developer"
            :disabled="row.role === 'admin' || row.is_system"
            @click="toggleDeveloper(row)"
          >
            {{ row.role === 'developer' ? t('admin.users.revokeDeveloper') : t('admin.users.grantDeveloper') }}
          </el-button>
          <el-button
            v-if="row.status === 'banned'"
            size="small"
            text
            data-testid="admin-users-unban"
            :disabled="!canModerate(row)"
            @click="unban(row)"
          >
            {{ t('admin.users.unban') }}
          </el-button>
          <el-button
            v-else
            size="small"
            text
            data-testid="admin-users-ban"
            :disabled="!canModerate(row)"
            @click="openBan(row)"
          >
            {{ t('admin.users.ban') }}
          </el-button>
          <el-button size="small" text data-testid="admin-users-uploads" @click="viewUploads(row)">
            {{ t('admin.users.viewUploads') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="pb-admin-module__pager"
      layout="prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="changePage"
    />

    <el-dialog
      :model-value="selected !== null"
      :title="t('admin.users.detailTitle', { name: selected?.username ?? '' })"
      width="720px"
      @update:model-value="closeDetail"
    >
      <el-descriptions v-if="selected" :column="1" border size="small">
        <el-descriptions-item :label="t('admin.common.id')">{{ selected.id }}</el-descriptions-item>
        <el-descriptions-item :label="t('admin.users.address')">
          {{ selected.email || t('admin.common.none') }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.users.role')">
          {{ roleText(selected.role) }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.users.status')">
          {{ statusText(selected.status) }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.users.systemAccount')">
          {{ selected.is_system ? t('admin.users.systemAccount') : t('admin.common.none') }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.users.banReason')">
          {{ selected.banned_reason || t('admin.common.none') }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.users.registeredAt')">
          {{ formatTime(selected.created_at) }}
        </el-descriptions-item>
      </el-descriptions>

      <el-tabs
        v-if="selected"
        v-model="detailTab"
        class="pb-admin-module__tabs"
        @tab-change="loadDetailTab"
      >
        <el-tab-pane :label="t('admin.users.tabRatings')" name="ratings">
          <p v-if="!detailLoading && ratings.length === 0" class="pb-admin-module__empty">
            {{ t('admin.users.ratingsEmpty') }}
          </p>
          <el-table v-if="ratings.length > 0" :data="ratings" size="small">
            <el-table-column :label="t('prompt.title')" min-width="200">
              <template #default="{ row }: { row: AdminUserRating }">{{ row.prompt_title }}</template>
            </el-table-column>
            <el-table-column :label="t('prompt.model')" min-width="140">
              <template #default="{ row }: { row: AdminUserRating }">
                {{ row.model_name || t('admin.common.none') }}
              </template>
            </el-table-column>
            <el-table-column :label="t('prompt.modelCompletion')" width="110">
              <template #default="{ row }: { row: AdminUserRating }">{{ row.score }}</template>
            </el-table-column>
            <el-table-column :label="t('library.revisions')" width="110">
              <template #default="{ row }: { row: AdminUserRating }">{{ row.edit_count }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.common.createdAt')" width="180">
              <template #default="{ row }: { row: AdminUserRating }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('admin.users.tabComments')" name="comments">
          <p v-if="!detailLoading && comments.length === 0" class="pb-admin-module__empty">
            {{ t('admin.users.commentsEmpty') }}
          </p>
          <el-table v-if="comments.length > 0" :data="comments" size="small">
            <el-table-column :label="t('prompt.title')" min-width="200">
              <template #default="{ row }: { row: AdminUserComment }">{{ row.prompt_title }}</template>
            </el-table-column>
            <el-table-column :label="t('library.fieldBody')" min-width="240">
              <template #default="{ row }: { row: AdminUserComment }">{{ row.body }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.common.createdAt')" width="180">
              <template #default="{ row }: { row: AdminUserComment }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('admin.prompts.deleted')" width="100">
              <template #default="{ row }: { row: AdminUserComment }">
                {{ row.deleted ? t('admin.prompts.deleted') : t('admin.common.none') }}
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('admin.users.tabLibraries')" name="libraries">
          <p v-if="!detailLoading && libraries.length === 0" class="pb-admin-module__empty">
            {{ t('admin.users.librariesEmpty') }}
          </p>
          <el-table v-if="libraries.length > 0" :data="libraries" size="small">
            <el-table-column :label="t('library.name')" min-width="200">
              <template #default="{ row }: { row: AdminUserLibrary }">{{ row.name }}</template>
            </el-table-column>
            <el-table-column :label="t('library.questions')" width="110">
              <template #default="{ row }: { row: AdminUserLibrary }">{{ row.question_count }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.prompts.deleted')" width="100">
              <template #default="{ row }: { row: AdminUserLibrary }">
                {{ row.deleted ? t('admin.prompts.deleted') : t('admin.common.none') }}
              </template>
            </el-table-column>
            <el-table-column :label="t('admin.common.createdAt')" width="180">
              <template #default="{ row }: { row: AdminUserLibrary }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane :label="t('admin.users.tabReports')" name="reports">
          <p v-if="!detailLoading && reports.length === 0" class="pb-admin-module__empty">
            {{ t('admin.users.reportsEmpty') }}
          </p>
          <el-table v-if="reports.length > 0" :data="reports" size="small">
            <el-table-column :label="t('admin.reports.objectType')" width="130">
              <template #default="{ row }: { row: AdminUserReport }">
                {{ objectTypeText(row.object_type) }}
              </template>
            </el-table-column>
            <el-table-column prop="object_id" :label="t('admin.common.id')" width="90" />
            <el-table-column :label="t('prompt.reportReason')" min-width="140">
              <template #default="{ row }: { row: AdminUserReport }">
                {{ reasonText(row.reason) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('admin.reports.status')" width="110">
              <template #default="{ row }: { row: AdminUserReport }">
                {{ reportStatusText(row.status) }}
              </template>
            </el-table-column>
            <el-table-column :label="t('admin.common.createdAt')" width="180">
              <template #default="{ row }: { row: AdminUserReport }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>

      <template #footer>
        <el-button data-testid="admin-users-close" @click="closeDetail">
          {{ t('admin.common.close') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      :model-value="banTarget !== null"
      :title="t('admin.users.ban')"
      width="480px"
      @update:model-value="closeBan"
    >
      <el-form label-position="top">
        <el-form-item :label="t('admin.users.banReason')">
          <el-input v-model="banReason" data-testid="admin-users-ban-reason" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="closeBan">{{ t('admin.common.cancel') }}</el-button>
        <el-button
          type="primary"
          :loading="busy"
          data-testid="admin-users-ban-confirm"
          @click="submitBan"
        >
          {{ t('admin.common.confirm') }}
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.pb-admin-module__head {
  align-items: baseline;
  display: flex;
  gap: var(--pb-space-md);
  justify-content: space-between;
}

.pb-admin-module__title {
  font-size: 18px;
  margin: 0;
}

.pb-admin-module__filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  margin: var(--pb-space-sm) 0;
}

.pb-admin-module__filter {
  max-width: 200px;
}

.pb-admin-module__error {
  color: var(--el-color-danger);
}

.pb-admin-module__empty {
  color: var(--pb-color-text-soft);
}

.pb-admin-module__pager {
  margin-top: var(--pb-space-sm);
}

.pb-admin-module__tabs {
  margin-top: var(--pb-space-sm);
}
</style>
