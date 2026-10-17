<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { type APITokenStatus, authApi } from '@/api/auth'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const activeTab = ref('profile')
const errorText = ref('')
const noticeText = ref('')

const exporting = ref(false)
const deleteConfirm = ref('')
const deleting = ref(false)
const deleteDialogVisible = ref(false)

const username = ref('')
const bio = ref('')
const avatarUrl = ref('')
const savingProfile = ref(false)

const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const savingPassword = ref(false)

const tokenStatus = ref<APITokenStatus>({ present: false })
const tokenPlain = ref('')
const tokenBusy = ref(false)

const canCopy =
  typeof navigator !== 'undefined' && typeof navigator.clipboard?.writeText === 'function'

const roleLabel = computed(() => (auth.user ? t(`role.${auth.user.role}`) : ''))
const statusLabel = computed(() => (auth.user ? t(`status.${auth.user.status}`) : ''))

onMounted(async () => {
  await loadProfile()
  await loadTokenStatus()
})

async function loadProfile(): Promise<void> {
  if (!auth.user) {
    await auth.fetchSession()
  }
  username.value = auth.user?.username ?? ''
  bio.value = auth.user?.bio ?? ''
  avatarUrl.value = auth.user?.avatar_url ?? ''
}

async function loadTokenStatus(): Promise<void> {
  try {
    tokenStatus.value = await authApi.apiTokenStatus()
  } catch (error) {
    errorText.value = messageOf(error)
  }
}

async function saveProfile(): Promise<void> {
  savingProfile.value = true
  errorText.value = ''
  noticeText.value = ''
  try {
    await auth.updateProfile({
      username: username.value,
      bio: bio.value,
      avatar_url: avatarUrl.value,
    })
    noticeText.value = t('me.saveSuccess')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    savingProfile.value = false
  }
}

async function changePassword(): Promise<void> {
  errorText.value = ''
  noticeText.value = ''
  if (newPassword.value !== confirmPassword.value) {
    errorText.value = t('auth.passwordMismatch')
    return
  }

  savingPassword.value = true
  try {
    await auth.changePassword(oldPassword.value, newPassword.value)
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    noticeText.value = t('me.passwordChanged')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    savingPassword.value = false
  }
}

async function createToken(): Promise<void> {
  tokenBusy.value = true
  errorText.value = ''
  noticeText.value = ''
  try {
    const created = await authApi.createApiToken()
    tokenPlain.value = created.token
    tokenStatus.value = { present: true, prefix: created.prefix }
    await loadTokenStatus()
    noticeText.value = t('me.tokenCreated')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    tokenBusy.value = false
  }
}

async function revokeToken(): Promise<void> {
  tokenBusy.value = true
  errorText.value = ''
  noticeText.value = ''
  try {
    await authApi.revokeApiToken()
    tokenPlain.value = ''
    await loadTokenStatus()
    noticeText.value = t('me.tokenRevoked')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    tokenBusy.value = false
  }
}

async function copyToken(): Promise<void> {
  if (!canCopy) {
    return
  }
  await navigator.clipboard.writeText(tokenPlain.value)
  noticeText.value = t('me.tokenCopied')
}

/** formatDate 把 RFC3339 时间转为本地可读形式。 */
function formatDate(raw: string): string {
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) {
    return raw
  }
  return parsed.toLocaleString()
}

/** exportData 下载「我的数据」JSON 文件（G-08）。 */
async function exportData(): Promise<void> {
  exporting.value = true
  errorText.value = ''
  noticeText.value = ''
  try {
    const exported = await authApi.exportMyData()
    const blob = new Blob([JSON.stringify(exported, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = exported.filename
    link.click()
    // 延迟撤销：Firefox 在下载尚未启动时撤销会中止保存动作。
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    noticeText.value = t('me.exportStarted')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    exporting.value = false
  }
}

/** openDeleteDialog 校验确认字符串后弹出二次确认；不一致时只提示不弹窗。 */
function openDeleteDialog(): void {
  errorText.value = ''
  noticeText.value = ''
  if (deleteConfirm.value !== (auth.user?.username ?? '')) {
    errorText.value = t('me.dangerMismatch')
    return
  }
  deleteDialogVisible.value = true
}

/** deleteAccount 注销账户（G-09）。成功后置为游客态并回到首页。 */
async function deleteAccount(): Promise<void> {
  deleting.value = true
  errorText.value = ''
  deleteDialogVisible.value = false
  try {
    await authApi.deleteAccount(deleteConfirm.value)
    auth.markGuest()
    await router.push('/')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <section class="pb-me">
    <header class="pb-me__header">
      <div class="pb-me__heading">
        <span class="pb-eyebrow" aria-hidden="true">Account</span>
        <h1 class="pb-me__title">{{ t('me.title') }}</h1>
      </div>
      <div class="pb-me__meta">
        <span data-testid="me-username">{{ auth.user?.username }}</span>
        <el-tag size="small" type="info">{{ roleLabel }}</el-tag>
        <el-tag size="small" :type="auth.user?.status === 'normal' ? 'success' : 'warning'">
          {{ statusLabel }}
        </el-tag>
      </div>
    </header>

    <el-alert
      v-if="auth.user && !auth.isVerified"
      class="pb-me__notice"
      type="warning"
      :title="t('me.unverifiedNotice')"
      show-icon
      data-testid="me-unverified"
    />
    <el-alert v-if="errorText" class="pb-me__notice" type="error" :title="errorText" show-icon />
    <el-alert
      v-else-if="noticeText"
      class="pb-me__notice"
      type="success"
      :title="noticeText"
      show-icon
      data-testid="me-notice"
    />

    <el-tabs v-model="activeTab">
      <el-tab-pane :label="t('me.profileTab')" name="profile">
        <el-form label-position="top" data-testid="me-profile-form" @submit.prevent="saveProfile">
          <el-form-item :label="t('me.username')">
            <el-input v-model="username" data-testid="me-username-input" />
          </el-form-item>
          <el-form-item :label="t('me.bio')">
            <el-input v-model="bio" type="textarea" :rows="3" data-testid="me-bio-input" />
          </el-form-item>
          <el-form-item :label="t('me.avatarUrl')">
            <el-input v-model="avatarUrl" data-testid="me-avatar-input" />
          </el-form-item>
          <el-button
            type="primary"
            native-type="submit"
            :loading="savingProfile"
            data-testid="me-save"
          >
            {{ t('me.save') }}
          </el-button>
        </el-form>
      </el-tab-pane>

      <el-tab-pane :label="t('me.passwordTab')" name="password">
        <el-form label-position="top" data-testid="me-password-form" @submit.prevent="changePassword">
          <el-form-item :label="t('auth.oldPassword')">
            <el-input
              v-model="oldPassword"
              type="password"
              show-password
              autocomplete="current-password"
              data-testid="me-old-password"
            />
          </el-form-item>
          <el-form-item :label="t('auth.newPassword')">
            <el-input
              v-model="newPassword"
              type="password"
              show-password
              autocomplete="new-password"
              data-testid="me-new-password"
            />
          </el-form-item>
          <el-form-item :label="t('auth.confirmPassword')">
            <el-input
              v-model="confirmPassword"
              type="password"
              show-password
              autocomplete="new-password"
              data-testid="me-confirm-password"
            />
          </el-form-item>
          <el-button
            type="primary"
            native-type="submit"
            :loading="savingPassword"
            data-testid="me-password-submit"
          >
            {{ t('me.changePassword') }}
          </el-button>
        </el-form>
      </el-tab-pane>

      <el-tab-pane :label="t('me.tokenTab')" name="token">
        <p class="pb-me__hint">{{ t('me.tokenHint') }}</p>
        <p v-if="tokenStatus.present" data-testid="me-token-present">
          {{ t('me.tokenPresent', { prefix: tokenStatus.prefix }) }}
          <span class="pb-me__token-meta">
            {{ t('me.tokenExpires', { date: formatDate(tokenStatus.expires_at ?? '') }) }}
            <em v-if="tokenStatus.expired" data-testid="me-token-expired">{{ t('me.tokenExpired') }}</em>
          </span>
        </p>
        <p v-else data-testid="me-token-absent">{{ t('me.tokenAbsent') }}</p>

        <div class="pb-me__actions">
          <el-button :loading="tokenBusy" data-testid="me-token-create" @click="createToken">
            {{ t('me.tokenCreate') }}
          </el-button>
          <el-button
            v-if="tokenStatus.present"
            :loading="tokenBusy"
            data-testid="me-token-revoke"
            @click="revokeToken"
          >
            {{ t('me.tokenRevoke') }}
          </el-button>
        </div>

        <div v-if="tokenPlain !== ''" class="pb-me__token">
          <el-input v-model="tokenPlain" readonly data-testid="me-token-plain" />
          <el-button v-if="canCopy" data-testid="me-token-copy" @click="copyToken">
            {{ t('me.tokenCopy') }}
          </el-button>
          <p class="pb-me__hint">{{ t('me.tokenCreatedHint') }}</p>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('me.dataTab')" name="data">
        <p class="pb-me__hint">{{ t('me.exportHint') }}</p>
        <div class="pb-me__actions">
          <el-button :loading="exporting" data-testid="me-export" @click="exportData">
            {{ t('me.exportButton') }}
          </el-button>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('me.dangerTab')" name="danger">
        <el-alert class="pb-me__notice" type="warning" :title="t('me.dangerHint')" show-icon />
        <el-form label-position="top" data-testid="me-delete-form" @submit.prevent="openDeleteDialog">
          <el-form-item :label="t('me.dangerConfirmLabel')">
            <el-input
              v-model="deleteConfirm"
              :placeholder="t('me.dangerConfirmPlaceholder')"
              data-testid="me-delete-confirm-input"
            />
          </el-form-item>
          <p class="pb-me__hint">{{ t('me.dangerWarning') }}</p>
          <el-button
            type="danger"
            native-type="submit"
            :loading="deleting"
            data-testid="me-delete-submit"
          >
            {{ t('me.dangerSubmit') }}
          </el-button>
        </el-form>
      </el-tab-pane>
    </el-tabs>

    <ConfirmDialog
      v-model="deleteDialogVisible"
      :title="t('me.dangerTab')"
      :message="t('me.dangerWarning')"
      :confirm-text="t('me.dangerSubmit')"
      :cancel-text="t('auth.cancel')"
      testid="me-delete"
      @confirm="deleteAccount"
    />
  </section>
</template>

<style scoped>
.pb-me {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  max-width: 840px;
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-me__header {
  position: relative;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  flex-wrap: wrap;
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-me__header::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 140px;
  height: 2px;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
}

.pb-me__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
}

.pb-me__title {
  font-size: 34px;
  font-weight: 500;
  letter-spacing: -0.012em;
  margin: 0;
}

.pb-me__meta {
  display: flex;
  align-items: center;
  gap: var(--pb-space-sm);
  color: var(--pb-ink-soft);
  font-size: 13.5px;
}

.pb-me__notice {
  margin: 0;
}

.pb-me__hint {
  color: var(--pb-ink-faint);
  font-size: 13px;
}

.pb-me__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
  margin: var(--pb-space-md) 0;
}

.pb-me__token {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  max-width: 560px;
}

.pb-me :deep(.el-tabs) {
  padding: var(--pb-space-sm) var(--pb-space-lg) var(--pb-space-lg);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
}

.pb-me :deep(.el-tabs__header) {
  margin-bottom: var(--pb-space-lg);
}

.pb-me :deep(.el-tabs__nav-wrap::after) {
  height: 1px;
  background: var(--pb-line);
}
</style>
