// 注册与邀请码管理（需求 §17.7）。
// 注册策略写入 registration_policy；邀请码的禁用只影响后续使用，不影响已注册用户。

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type AdminInvite,
  type AdminInviteUsage,
  adminApi,
  type RegistrationPolicy,
} from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { formatDateTime } from '@/utils/datetime'

const { t, locale } = useI18n()
const { messageOf } = useErrorMessage()

const PAGE_SIZE = 20

const policy = ref<RegistrationPolicy>('open')
const rows = ref<AdminInvite[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const failure = ref('')

const filters = ref({ kind: '', disabled: '' })

/** GenerateForm 是生成对话框的输入取值。 */
interface GenerateForm {
  kind: AdminInvite['kind']
  count: number
  maxUses: number
  expiresInDays: number
  note: string
}

const generateVisible = ref(false)
const generateForm = ref<GenerateForm>({
  kind: 'normal',
  count: 1,
  maxUses: 1,
  expiresInDays: 0,
  note: '',
})

const usagesInvite = ref<AdminInvite | null>(null)
const usages = ref<AdminInviteUsage[]>([])
const usagesTotal = ref(0)
const usagesPage = ref(1)
const usagesLoading = ref(false)

/** kindLabel 返回邀请码类型的界面文案。 */
function kindLabel(kind: AdminInvite['kind']): string {
  return kind === 'normal' ? t('admin.invites.kindNormal') : t('admin.invites.kindDeveloper')
}

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw?: string): string {
  if (raw === undefined || raw === '') {
    return ''
  }
  return formatDateTime(raw, locale.value)
}

/** formatExpires 呈现过期时刻；未设置过期时间时显示永不过期。 */
function formatExpires(raw?: string): string {
  if (raw === undefined || raw === '') {
    return t('admin.invites.expiresNever')
  }
  return formatTime(raw)
}

async function loadPolicy(): Promise<void> {
  try {
    const result = await adminApi.registrationPolicy()
    policy.value = result.policy
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function savePolicy(): Promise<void> {
  try {
    await adminApi.saveSetting('registration_policy', policy.value)
  } catch (error) {
    failure.value = messageOf(error)
    return
  }
  ElMessage.success(t('admin.invites.policySaved'))
}

async function loadInvites(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.invites({
      page: page.value,
      page_size: PAGE_SIZE,
      kind: filters.value.kind,
      disabled: filters.value.disabled,
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

function search(): void {
  page.value = 1
  void loadInvites()
}

function resetFilters(): void {
  filters.value = { kind: '', disabled: '' }
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadInvites()
}

function openGenerate(): void {
  generateForm.value = { kind: 'normal', count: 1, maxUses: 1, expiresInDays: 0, note: '' }
  generateVisible.value = true
}

async function submitGenerate(): Promise<void> {
  const form = generateForm.value
  try {
    await adminApi.createInvites({
      kind: form.kind,
      count: form.count,
      max_uses: form.maxUses,
      expires_in_days: form.expiresInDays,
      note: form.note,
    })
  } catch (error) {
    failure.value = messageOf(error)
    return
  }
  ElMessage.success(t('admin.invites.generated', { count: form.count }))
  generateVisible.value = false
  await loadInvites()
}

async function toggleDisabled(invite: AdminInvite): Promise<void> {
  try {
    await adminApi.setInviteDisabled(invite.id, !invite.disabled)
  } catch (error) {
    failure.value = messageOf(error)
    return
  }
  ElMessage.success(t('admin.invites.stateChanged'))
  await loadInvites()
}

/** copyCode 复制邀请码；浏览器没有提供剪贴板接口时跳过。 */
async function copyCode(code: string): Promise<void> {
  const clipboard: Clipboard | undefined = navigator.clipboard
  if (clipboard === undefined) {
    return
  }
  await clipboard.writeText(code)
  ElMessage.success(t('admin.common.copied'))
}

async function openUsages(invite: AdminInvite): Promise<void> {
  usagesInvite.value = invite
  usagesPage.value = 1
  await loadUsages()
}

async function loadUsages(): Promise<void> {
  const invite = usagesInvite.value
  if (invite === null) {
    return
  }
  usagesLoading.value = true
  try {
    const result = await adminApi.inviteUsages(invite.id, usagesPage.value, PAGE_SIZE)
    usages.value = result.items
    usagesTotal.value = result.total
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    usagesLoading.value = false
  }
}

function closeUsages(): void {
  usagesInvite.value = null
  usages.value = []
  usagesTotal.value = 0
  usagesPage.value = 1
}

function changeUsagesPage(next: number): void {
  usagesPage.value = next
  void loadUsages()
}

onMounted(async () => {
  await loadPolicy()
  await loadInvites()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title">{{ t('admin.invites.title') }}</h2>
    </header>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-invites-error">
      {{ failure }}
    </p>

    <div class="pb-admin-module__policy" data-testid="admin-invites-policy">
      <div class="pb-admin-module__policy-row">
        <span class="pb-admin-module__policy-label">{{ t('admin.invites.policy') }}</span>
        <el-radio-group v-model="policy">
          <el-radio value="open">{{ t('admin.invites.policyOpen') }}</el-radio>
          <el-radio value="closed">{{ t('admin.invites.policyClosed') }}</el-radio>
          <el-radio value="invite_only">{{ t('admin.invites.policyInviteOnly') }}</el-radio>
        </el-radio-group>
        <el-button
          type="primary"
          size="small"
          data-testid="admin-invites-policy-save"
          @click="savePolicy"
        >
          {{ t('admin.common.save') }}
        </el-button>
      </div>
      <p class="pb-admin-module__hint">{{ t('admin.invites.policyHint') }}</p>
    </div>

    <div class="pb-admin-module__filters">
      <el-select
        v-model="filters.kind"
        class="pb-admin-module__filter"
        :placeholder="t('admin.invites.kind')"
        data-testid="admin-invites-kind"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('admin.invites.kindNormal')" value="normal" />
        <el-option :label="t('admin.invites.kindDeveloper')" value="developer" />
      </el-select>
      <el-select
        v-model="filters.disabled"
        class="pb-admin-module__filter"
        :placeholder="t('admin.invites.disabledState')"
        data-testid="admin-invites-disabled"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('admin.common.disabled')" value="true" />
        <el-option :label="t('admin.common.enabled')" value="false" />
      </el-select>
      <el-button type="primary" size="small" data-testid="admin-invites-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-invites-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
      <el-button size="small" data-testid="admin-invites-generate" @click="openGenerate">
        {{ t('admin.invites.generate') }}
      </el-button>
    </div>

    <p
      v-if="!loading && rows.length === 0"
      class="pb-admin-module__empty"
      data-testid="admin-invites-empty"
    >
      {{ t('admin.common.empty') }}
    </p>

    <el-table v-if="rows.length > 0" :data="rows" size="small" data-testid="admin-invites-table">
      <el-table-column :label="t('admin.invites.code')" min-width="220">
        <template #default="{ row }: { row: AdminInvite }">
          <span class="pb-admin-module__code">{{ row.code }}</span>
          <el-button size="small" text @click="copyCode(row.code)">
            {{ t('admin.common.copied') }}
          </el-button>
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.invites.kind')" width="100">
        <template #default="{ row }: { row: AdminInvite }">{{ kindLabel(row.kind) }}</template>
      </el-table-column>
      <el-table-column prop="creator_name" :label="t('admin.invites.creator')" width="140" />
      <el-table-column :label="t('admin.invites.usedCount')" width="100">
        <template #default="{ row }: { row: AdminInvite }">
          {{ row.used_count }}/{{ row.max_uses }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.invites.expiresInDays')" width="180">
        <template #default="{ row }: { row: AdminInvite }">
          {{ formatExpires(row.expires_at) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.invites.disabledState')" width="100">
        <template #default="{ row }: { row: AdminInvite }">
          <el-tag :type="row.disabled ? 'info' : 'success'" size="small">
            {{ row.disabled ? t('admin.common.disabled') : t('admin.common.enabled') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.common.createdAt')" width="180">
        <template #default="{ row }: { row: AdminInvite }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('admin.common.actions')" width="170">
        <template #default="{ row }: { row: AdminInvite }">
          <el-button size="small" text data-testid="admin-invites-usages" @click="openUsages(row)">
            {{ t('admin.invites.usages') }}
          </el-button>
          <el-button size="small" text data-testid="admin-invites-disable" @click="toggleDisabled(row)">
            {{ row.disabled ? t('admin.common.enable') : t('admin.common.disable') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="pb-admin-module__pager"
      layout="prev, pager, next"
      :current-page="page"
      :page-size="PAGE_SIZE"
      :total="total"
      @current-change="changePage"
    />

    <el-dialog
      v-model="generateVisible"
      :title="t('admin.invites.generateTitle')"
      width="520px"
    >
      <el-form size="small">
        <el-form-item :label="t('admin.invites.kind')">
          <el-select v-model="generateForm.kind" data-testid="admin-invites-form-kind">
            <el-option :label="t('admin.invites.kindNormal')" value="normal" />
            <el-option :label="t('admin.invites.kindDeveloper')" value="developer" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('admin.invites.count')">
          <el-input-number
            v-model="generateForm.count"
            :min="1"
            :max="50"
            data-testid="admin-invites-form-count"
          />
        </el-form-item>
        <el-form-item :label="t('admin.invites.maxUses')">
          <el-input-number
            v-model="generateForm.maxUses"
            :min="1"
            :max="1000"
            data-testid="admin-invites-form-max-uses"
          />
        </el-form-item>
        <el-form-item :label="t('admin.invites.expiresInDays')">
          <el-input-number
            v-model="generateForm.expiresInDays"
            :min="0"
            :max="365"
            data-testid="admin-invites-form-expires"
          />
          <span class="pb-admin-module__hint">{{ t('admin.invites.expiresNever') }}</span>
        </el-form-item>
        <el-form-item>
          <el-input
            v-model="generateForm.note"
            type="textarea"
            :rows="3"
            :maxlength="200"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="generateVisible = false">{{ t('admin.common.cancel') }}</el-button>
        <el-button
          type="primary"
          data-testid="admin-invites-form-submit"
          @click="submitGenerate"
        >
          {{ t('admin.common.confirm') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      :model-value="usagesInvite !== null"
      :title="t('admin.invites.usagesTitle', { code: usagesInvite?.code ?? '' })"
      width="640px"
      @update:model-value="closeUsages"
    >
      <p v-if="!usagesLoading && usages.length === 0" class="pb-admin-module__empty">
        {{ t('admin.invites.usagesEmpty') }}
      </p>
      <el-table v-if="usages.length > 0" :data="usages" size="small">
        <el-table-column prop="username" :label="t('admin.invites.user')" min-width="160" />
        <el-table-column prop="user_id" :label="t('admin.common.id')" width="100" />
        <el-table-column :label="t('admin.invites.usedAt')" width="180">
          <template #default="{ row }: { row: AdminInviteUsage }">
            {{ formatTime(row.used_at) }}
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="pb-admin-module__pager"
        layout="prev, pager, next"
        :current-page="usagesPage"
        :page-size="PAGE_SIZE"
        :total="usagesTotal"
        @current-change="changeUsagesPage"
      />
      <template #footer>
        <el-button @click="closeUsages">{{ t('admin.common.close') }}</el-button>
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

.pb-admin-module__hint {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  margin: var(--pb-space-xs) 0;
}

.pb-admin-module__policy {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 4px;
  margin: var(--pb-space-sm) 0;
  padding: var(--pb-space-sm);
}

.pb-admin-module__policy-row {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
}

.pb-admin-module__policy-label {
  color: var(--pb-color-text-soft);
  font-size: 13px;
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

.pb-admin-module__code {
  font-family: monospace;
  font-size: 13px;
  margin-right: var(--pb-space-xs);
}

.pb-admin-module__pager {
  margin-top: var(--pb-space-sm);
}
</style>
