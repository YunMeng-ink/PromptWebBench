<script setup lang="ts">
// 私有题库列表：创建题库、进入题库、管理回收站中的题库。
// 权限按钮按后端返回的角色显示，真正的判定仍在后端。

import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { type Library, libraryApi, type RecycledLibrary } from '@/api/library'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { roleKey } from '@/views/libraryText'

const router = useRouter()
const { t } = useI18n()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const libraries = ref<Library[]>([])
const recycled = ref<RecycledLibrary[]>([])
const loading = ref(true)
const failure = ref('')
const notice = ref('')

const dialogVisible = ref(false)
const form = ref({ name: '', description: '' })

/** purgeTarget 是等待二次确认的回收站题库。 */
const purgeTarget = ref<RecycledLibrary | null>(null)

async function load(): Promise<void> {
  loading.value = true
  try {
    const [mine, bin] = await Promise.all([libraryApi.mine(1, 50), libraryApi.recycleBin()])
    libraries.value = mine.items
    recycled.value = bin.items
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  form.value = { name: '', description: '' }
  dialogVisible.value = true
}

async function submitCreate(): Promise<void> {
  try {
    const created = await libraryApi.create(form.value.name, form.value.description)
    dialogVisible.value = false
    notice.value = t('library.created')
    await load()
    await router.push(`/libraries/${created.library.id}`)
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function restore(entry: RecycledLibrary): Promise<void> {
  try {
    await libraryApi.restore(entry.library_id)
    notice.value = t('library.restored')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function purge(entry: RecycledLibrary): Promise<void> {
  purgeTarget.value = entry
}

/** confirmPurge 执行已被确认的彻底删除。 */
async function confirmPurge(): Promise<void> {
  const entry = purgeTarget.value
  purgeTarget.value = null
  if (entry === null) {
    return
  }
  try {
    await libraryApi.purge(entry.library_id)
    notice.value = t('library.purged')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

onMounted(async () => {
  await auth.connect()
  await load()
})
</script>

<template>
  <section class="pb-libraries">
    <header class="pb-libraries__head">
      <div class="pb-libraries__heading">
        <span class="pb-eyebrow" aria-hidden="true">Collections</span>
        <h1 class="pb-libraries__title" data-testid="libraries-title">{{ t('library.title') }}</h1>
      </div>
      <el-button
        v-if="auth.isAuthenticated"
        type="primary"
        data-testid="library-create"
        @click="openCreate"
      >
        {{ t('library.create') }}
      </el-button>
    </header>

    <p v-if="failure" class="pb-libraries__error" data-testid="libraries-error">{{ failure }}</p>
    <p v-if="notice" class="pb-libraries__notice" data-testid="libraries-notice">{{ notice }}</p>

    <p v-if="loading" class="pb-libraries__hint">{{ t('library.title') }}</p>
    <div v-else-if="libraries.length === 0" class="pb-empty" data-testid="libraries-empty">
      <span class="pb-empty__title">{{ t('library.empty') }}</span>
    </div>
    <ul v-else class="pb-libraries__list" data-testid="libraries-list">
      <li
        v-for="library in libraries"
        :key="library.id"
        class="pb-libraries__entry"
        :data-testid="`library-${library.id}`"
      >
        <RouterLink class="pb-libraries__name" :to="`/libraries/${library.id}`">
          {{ library.name }}
        </RouterLink>
        <span class="pb-libraries__meta" :data-testid="`library-role-${library.id}`">
          {{ t(roleKey(library.role)) }} ·
          {{ t('library.capacity', { used: library.question_count, limit: library.question_limit }) }}
          <template v-if="library.owner_name">
            · {{ t('library.owner', { name: library.owner_name }) }}
          </template>
        </span>
        <p class="pb-libraries__description">{{ library.description }}</p>
      </li>
    </ul>

    <section class="pb-libraries__recycle">
      <h2 class="pb-libraries__subtitle">{{ t('library.recycleBin') }}</h2>
      <p v-if="recycled.length === 0" class="pb-libraries__meta" data-testid="libraries-recycle-empty">
        {{ t('library.recycleBinEmpty') }}
      </p>
      <ul v-else data-testid="libraries-recycle">
        <li
          v-for="entry in recycled"
          :key="entry.library_id"
          class="pb-libraries__entry"
          :data-testid="`library-recycled-${entry.library_id}`"
        >
          <span class="pb-libraries__name">{{ entry.name }}</span>
          <span class="pb-libraries__meta">{{ entry.expires_at }}</span>
          <div class="pb-libraries__entry-actions">
            <el-button
              size="small"
              :data-testid="`library-restore-${entry.library_id}`"
              @click="restore(entry)"
            >
              {{ t('library.restore') }}
            </el-button>
            <el-button
              size="small"
              type="danger"
              plain
              :data-testid="`library-purge-${entry.library_id}`"
              @click="purge(entry)"
            >
              {{ t('library.purge') }}
            </el-button>
          </div>
        </li>
      </ul>
    </section>

    <el-dialog v-model="dialogVisible" :title="t('library.create')" width="440px">
      <el-form label-width="90px">
        <el-form-item :label="t('library.name')">
          <el-input v-model="form.name" maxlength="100" data-testid="library-new-name" />
        </el-form-item>
        <el-form-item :label="t('library.description')">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
            maxlength="2000"
            data-testid="library-new-description"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button data-testid="library-create-cancel" @click="dialogVisible = false">
          {{ t('auth.cancel') }}
        </el-button>
        <el-button type="primary" data-testid="library-create-submit" @click="submitCreate">
          {{ t('library.save') }}
        </el-button>
      </template>
    </el-dialog>

    <ConfirmDialog
      :model-value="purgeTarget !== null"
      :title="t('library.purge')"
      :message="t('library.purgeConfirm')"
      :confirm-text="t('library.purge')"
      :cancel-text="t('auth.cancel')"
      testid="library-purge"
      @update:model-value="
        (value: boolean) => {
          if (!value) purgeTarget = null
        }
      "
      @confirm="confirmPurge"
    />
  </section>
</template>

<style scoped>
.pb-libraries {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-libraries__head {
  position: relative;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-libraries__head::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 140px;
  height: 2px;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
}

.pb-libraries__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
}

.pb-libraries__title {
  font-size: 34px;
  font-weight: 500;
  letter-spacing: -0.012em;
  margin: 0;
}

.pb-libraries__subtitle {
  margin: 0 0 var(--pb-space-sm);
  font-size: 18px;
  font-weight: 600;
}

.pb-libraries__list,
.pb-libraries__recycle ul {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-libraries__entry {
  padding: var(--pb-space-md) var(--pb-space-lg);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  transition:
    border-color var(--pb-duration) var(--pb-ease),
    box-shadow var(--pb-duration) var(--pb-ease);
}

.pb-libraries__entry:hover {
  border-color: var(--pb-line-strong);
  box-shadow: var(--pb-shadow-sm);
}

.pb-libraries__name {
  color: var(--pb-ink);
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.006em;
}

a.pb-libraries__name:hover {
  color: var(--pb-accent);
}

.pb-libraries__meta,
.pb-libraries__description {
  display: block;
  margin: 4px 0 0;
  color: var(--pb-ink-faint);
  font-size: 12.5px;
}

.pb-libraries__entry-actions {
  display: flex;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-sm);
}

.pb-libraries__recycle {
  padding-top: var(--pb-space-lg);
  border-top: 1px solid var(--pb-line);
}

.pb-libraries__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

.pb-libraries__notice,
.pb-libraries__hint {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
}
</style>
