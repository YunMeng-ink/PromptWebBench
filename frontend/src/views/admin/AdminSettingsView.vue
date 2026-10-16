// 系统设置（需求 §20）。
// 密钥项只写不读：界面不回显任何取值，未配置主密钥时后端拒绝保存并返回 secret_unavailable。

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type AdminConfigValue, adminApi, type ConfigKind } from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()
const { messageOf } = useErrorMessage()

/** SettingRow 承载配置项与界面上的编辑草稿。 */
interface SettingRow extends AdminConfigValue {
  numberDraft: number | undefined
  flagDraft: boolean
  textDraft: string
  error: string
}

const rows = ref<SettingRow[]>([])
/**
 * secretWritable 反映后端是否配置了主密钥。
 * 未配置时密钥类配置项无法保存，页面据此隐藏那段提示之外的失败路径。
 */
const secretWritable = ref(true)
const failure = ref('')
const testEmailTo = ref('')

const smtpRows = computed(() => rows.value.filter((row) => row.key.startsWith('smtp_')))
const businessRows = computed(() => rows.value.filter((row) => !row.key.startsWith('smtp_')))

/** groups 按 key 前缀把配置项分成邮件与业务两组。 */
const groups = computed(() => [
  {
    testid: 'admin-settings-group-smtp',
    title: t('admin.settings.groupSMTP'),
    rows: smtpRows.value,
  },
  {
    testid: 'admin-settings-group-business',
    title: t('admin.settings.groupBusiness'),
    rows: businessRows.value,
  },
])

/** draftNumber 取数值草稿；生效值来自环境变量时是字符串，需要转换。 */
function draftNumber(raw: unknown): number | undefined {
  if (typeof raw === 'number') {
    return raw
  }
  if (typeof raw === 'string' && raw.trim() !== '') {
    const parsed = Number(raw)
    return Number.isNaN(parsed) ? undefined : parsed
  }
  return undefined
}

/** draftFlag 取开关草稿；环境变量给出的布尔值是字符串。 */
function draftFlag(raw: unknown): boolean {
  return raw === true || raw === 'true'
}

/** draftList 把取值渲染成每行一项的文本。 */
function draftList(raw: unknown): string {
  if (Array.isArray(raw)) {
    const list: unknown[] = raw
    return list.map((item) => String(item)).join('\n')
  }
  return typeof raw === 'string' ? raw : ''
}

/** draftText 取字符串类配置的编辑草稿。 */
function draftText(kind: ConfigKind, raw: unknown): string {
  return kind === 'string_list' ? draftList(raw) : typeof raw === 'string' ? raw : ''
}

function toRow(item: AdminConfigValue): SettingRow {
  const raw = item.value ?? item.effective
  return {
    ...item,
    numberDraft: draftNumber(raw),
    flagDraft: draftFlag(raw),
    textDraft: item.secret ? '' : draftText(item.kind, raw),
    error: '',
  }
}

/** sourceLabel 把取值来源翻译成文案。 */
function sourceLabel(row: SettingRow): string {
  if (row.source === 'env') {
    return t('admin.settings.sourceEnv')
  }
  if (row.source === 'config') {
    return t('admin.settings.sourceConfig')
  }
  return t('admin.common.none')
}

/** secretState 按后端给出的取值与来源判断密钥是否已设置。 */
function secretState(row: SettingRow): string {
  const configured = row.value !== undefined || row.source === 'config'
  return configured ? t('admin.settings.secretSet') : t('admin.settings.secretNotSet')
}

/** effectiveText 呈现生效值；未配置时显示无。 */
function effectiveText(value: unknown): string {
  if (value === undefined || value === null) {
    return t('admin.common.none')
  }
  if (typeof value === 'string') {
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  return JSON.stringify(value, null, 2)
}

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw?: string): string {
  if (raw === undefined || raw === '') {
    return ''
  }
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? raw : parsed.toLocaleString()
}

/** enumOptions 返回枚举配置项的候选取值；目前只有注册策略是枚举。 */
function enumOptions(key: string): { value: string; label: string }[] {
  if (key !== 'registration_policy') {
    return []
  }
  return [
    { value: 'open', label: t('admin.invites.policyOpen') },
    { value: 'closed', label: t('admin.invites.policyClosed') },
    { value: 'invite_only', label: t('admin.invites.policyInviteOnly') },
  ]
}

async function load(): Promise<void> {
  try {
    const result = await adminApi.settings()
    rows.value = result.items.map(toRow)
    secretWritable.value = result.secret_writable
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function save(row: SettingRow): Promise<void> {
  row.error = ''
  let value: unknown
  if (row.kind === 'int' || row.kind === 'float') {
    if (row.numberDraft === undefined || Number.isNaN(row.numberDraft)) {
      row.error = t('admin.settings.invalidValue')
      return
    }
    value = row.kind === 'int' ? Math.trunc(Number(row.numberDraft)) : Number(row.numberDraft)
  } else if (row.kind === 'string_list') {
    value = row.textDraft
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '')
  } else if (row.kind === 'bool') {
    value = row.flagDraft
  } else {
    value = row.textDraft
  }
  try {
    await adminApi.saveSetting(row.key, value)
  } catch (error) {
    row.error = messageOf(error)
    ElMessage.error(row.error)
    return
  }
  ElMessage.success(t('admin.settings.saved'))
  await load()
}

async function sendTestEmail(): Promise<void> {
  try {
    await adminApi.sendTestEmail(testEmailTo.value)
  } catch (error) {
    const text = `${t('admin.settings.testEmailFailed')}：${messageOf(error)}`
    failure.value = text
    ElMessage.error(text)
    return
  }
  failure.value = ''
  ElMessage.success(t('admin.settings.testEmailSent'))
}

onMounted(async () => {
  await load()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title">{{ t('admin.settings.title') }}</h2>
    </header>

    <p class="pb-admin-module__hint">{{ t('admin.settings.secretHint') }}</p>
    <p class="pb-admin-module__hint">{{ t('admin.settings.envHint') }}</p>
    <p v-if="!secretWritable" class="pb-admin-module__hint" data-testid="admin-settings-no-master-key">
      {{ t('admin.settings.noMasterKeyHint') }}
    </p>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-settings-error">
      {{ failure }}
    </p>

    <el-card
      v-for="group in groups"
      :key="group.testid"
      class="pb-admin-settings__group"
      :data-testid="group.testid"
    >
      <template #header>{{ group.title }}</template>
      <p v-if="group.rows.length === 0" class="pb-admin-module__empty">
        {{ t('admin.common.empty') }}
      </p>
      <div v-for="row in group.rows" :key="row.key" class="pb-admin-settings__row">
        <div class="pb-admin-settings__main">
          <div class="pb-admin-settings__info">
            <div class="pb-admin-settings__desc">{{ row.description }}</div>
            <div class="pb-admin-settings__key">{{ row.key }}</div>
            <div class="pb-admin-settings__meta">
              <span>{{ t('admin.settings.source') }}：{{ sourceLabel(row) }}</span>
              <span v-if="row.secret">
                {{ t('admin.settings.secret') }}：{{ secretState(row) }}
              </span>
              <span v-else>
                {{ t('admin.settings.effectiveValue') }}：{{ effectiveText(row.effective) }}
              </span>
              <span v-if="row.updated_at">
                {{ t('admin.settings.updatedAt') }}：{{ formatTime(row.updated_at) }}
              </span>
            </div>
          </div>
          <div class="pb-admin-settings__edit">
            <el-input-number
              v-if="row.kind === 'int'"
              v-model="row.numberDraft"
              :precision="0"
              :data-testid="'admin-settings-value-' + row.key"
            />
            <el-input-number
              v-else-if="row.kind === 'float'"
              v-model="row.numberDraft"
              :data-testid="'admin-settings-value-' + row.key"
            />
            <el-switch
              v-else-if="row.kind === 'bool'"
              v-model="row.flagDraft"
              :data-testid="'admin-settings-value-' + row.key"
            />
            <el-select
              v-else-if="row.kind === 'enum'"
              v-model="row.textDraft"
              :data-testid="'admin-settings-value-' + row.key"
            >
              <el-option
                v-for="option in enumOptions(row.key)"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
            <el-input
              v-else-if="row.kind === 'string_list'"
              v-model="row.textDraft"
              type="textarea"
              :rows="3"
              :placeholder="t('admin.settings.listPlaceholder')"
              :data-testid="'admin-settings-value-' + row.key"
            />
            <el-input
              v-else
              v-model="row.textDraft"
              :type="row.secret ? 'password' : 'text'"
              :show-password="row.secret"
              :placeholder="
                row.secret ? t('admin.settings.secretHint') : t('admin.settings.valuePlaceholder')
              "
              :data-testid="'admin-settings-value-' + row.key"
            />
            <el-button
              type="primary"
              size="small"
              :data-testid="'admin-settings-save-' + row.key"
              @click="save(row)"
            >
              {{ t('admin.common.save') }}
            </el-button>
          </div>
        </div>
        <p v-if="row.error" class="pb-admin-settings__row-error">{{ row.error }}</p>
      </div>
    </el-card>

    <el-card class="pb-admin-settings__group">
      <template #header>{{ t('admin.settings.testEmail') }}</template>
      <div class="pb-admin-settings__row pb-admin-settings__row--test">
        <span class="pb-admin-settings__label">{{ t('admin.settings.testEmailTo') }}</span>
        <el-input
          v-model="testEmailTo"
          class="pb-admin-settings__test-input"
          :placeholder="t('admin.settings.testEmailTo')"
          data-testid="admin-settings-test-to"
        />
        <el-button
          type="primary"
          size="small"
          data-testid="admin-settings-test-send"
          @click="sendTestEmail"
        >
          {{ t('admin.settings.testEmail') }}
        </el-button>
      </div>
    </el-card>
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

.pb-admin-module__error {
  color: var(--el-color-danger);
}

.pb-admin-module__empty {
  color: var(--pb-color-text-soft);
}

.pb-admin-settings__group {
  margin-bottom: var(--pb-space-sm);
}

.pb-admin-settings__row {
  border-top: 1px solid var(--el-border-color-lighter);
  padding: var(--pb-space-sm) 0;
}

.pb-admin-settings__row:first-child {
  border-top: none;
}

.pb-admin-settings__main {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
  justify-content: space-between;
}

.pb-admin-settings__info {
  flex: 1 1 320px;
}

.pb-admin-settings__desc {
  font-size: 14px;
}

.pb-admin-settings__key {
  color: var(--pb-color-text-faint);
  font-family: monospace;
  font-size: 12px;
}

.pb-admin-settings__meta {
  color: var(--pb-color-text-soft);
  display: flex;
  flex-wrap: wrap;
  font-size: 12px;
  gap: var(--pb-space-xs);
}

.pb-admin-settings__edit {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-admin-settings__row-error {
  color: var(--el-color-danger);
  font-size: 12px;
  margin: var(--pb-space-xs) 0 0;
}

.pb-admin-settings__label {
  color: var(--pb-color-text-soft);
  font-size: 13px;
}

.pb-admin-settings__test-input {
  max-width: 280px;
}
</style>
