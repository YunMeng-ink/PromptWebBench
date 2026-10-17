// 收藏夹：创建、改名、删除、浏览条目，以及对关注对象的公开收藏夹的访问。
// 条目列表由后端逐条复查可见性，前端不做二次判断。

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { type Favorite, type FavoriteItem, socialApi } from '@/api/social'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const { t, locale } = useI18n()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

/** ownerId 为「查看他人收藏夹」模式下的目标用户；为 null 表示查看自己的。 */
const ownerId = computed(() => {
  const raw = route.params.userId
  if (raw === undefined) {
    return null
  }
  const parsed = Number.parseInt(String(raw), 10)
  return Number.isNaN(parsed) ? null : parsed
})

const viewingSelf = computed(() => ownerId.value === null || ownerId.value === auth.user?.id)

const favorites = ref<Favorite[]>([])
const selected = ref<Favorite | null>(null)
const items = ref<FavoriteItem[]>([])
const itemsTotal = ref(0)
const itemsPage = ref(1)
const pageSize = 20

const loading = ref(true)
const failure = ref('')
const notice = ref('')

const dialogVisible = ref(false)
const editing = ref<Favorite | null>(null)
const form = ref({ name: '', description: '', visibility: 'private' as 'public' | 'private' })

/** createDialog 打开新建对话框。 */
function createDialog(): void {
  editing.value = null
  form.value = { name: '', description: '', visibility: 'private' }
  dialogVisible.value = true
}

/** editDialog 打开改名对话框。 */
function editDialog(favorite: Favorite): void {
  editing.value = favorite
  form.value = {
    name: favorite.name,
    description: favorite.description,
    visibility: favorite.visibility,
  }
  dialogVisible.value = true
}

async function loadFavorites(): Promise<void> {
  loading.value = true
  try {
    const result =
      ownerId.value === null
        ? await socialApi.myFavorites(1, 50)
        : await socialApi.favoritesOf(ownerId.value, 1, 50)
    favorites.value = result.items
    failure.value = ''
    if (favorites.value.length > 0) {
      await selectFavorite(favorites.value[0] as Favorite)
    } else {
      selected.value = null
      items.value = []
      itemsTotal.value = 0
    }
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

async function selectFavorite(favorite: Favorite): Promise<void> {
  selected.value = favorite
  itemsPage.value = 1
  await loadItems()
}

async function loadItems(): Promise<void> {
  if (selected.value === null) {
    return
  }
  try {
    const result = await socialApi.favoriteItems(selected.value.id, itemsPage.value, pageSize)
    items.value = result.items
    itemsTotal.value = result.total
  } catch (error) {
    failure.value = messageOf(error)
    items.value = []
    itemsTotal.value = 0
  }
}

async function changeItemsPage(next: number): Promise<void> {
  itemsPage.value = next
  await loadItems()
}

async function submitForm(): Promise<void> {
  try {
    if (editing.value === null) {
      await socialApi.createFavorite(form.value)
      notice.value = t('prompt.favoriteCreated')
    } else {
      await socialApi.updateFavorite(editing.value.id, form.value)
      notice.value = t('prompt.updated')
    }
    dialogVisible.value = false
    await loadFavorites()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function removeFavorite(favorite: Favorite): Promise<void> {
  try {
    await socialApi.removeFavorite(favorite.id)
    notice.value = t('prompt.favoriteDeleted')
    await loadFavorites()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function removeItem(item: FavoriteItem): Promise<void> {
  if (selected.value === null) {
    return
  }
  try {
    await socialApi.removeItem(selected.value.id, item.prompt_id)
    notice.value = t('prompt.favoriteRemoved')
    await loadFavorites()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** localized 按当前语言选择考察目的或考察维度名称。 */
function localized(zh: string, en: string): string {
  return locale.value === 'en' ? en : zh
}

onMounted(async () => {
  await auth.connect()
  await loadFavorites()
})

// 同路由参数切换（他人收藏夹之间）时组件复用：复位全部状态并重载。
watch(ownerId, () => {
  favorites.value = []
  selected.value = null
  items.value = []
  itemsTotal.value = 0
  itemsPage.value = 1
  loading.value = true
  failure.value = ''
  notice.value = ''
  void loadFavorites()
})
</script>

<template>
  <section class="pb-favorites">
    <header class="pb-favorites__head">
      <div class="pb-favorites__heading">
        <span class="pb-eyebrow" aria-hidden="true">Favorites</span>
        <h1 class="pb-favorites__title" data-testid="favorites-title">
          {{ viewingSelf ? t('prompt.favoriteManage') : t('prompt.profileTitle') }}
        </h1>
      </div>
      <el-button
        v-if="viewingSelf && auth.isAuthenticated"
        type="primary"
        data-testid="favorites-create"
        @click="createDialog"
      >
        {{ t('prompt.favoriteCreateHint') }}
      </el-button>
    </header>

    <p v-if="failure" class="pb-favorites__error" data-testid="favorites-error">{{ failure }}</p>
    <p v-if="notice" class="pb-favorites__notice" data-testid="favorites-notice">{{ notice }}</p>

    <p v-if="loading" class="pb-favorites__hint">{{ t('prompt.favoriteEmpty') }}</p>
    <div v-else-if="favorites.length === 0" class="pb-empty" data-testid="favorites-empty">
      <span class="pb-empty__title">{{ t('prompt.favoriteEmpty') }}</span>
    </div>

    <div v-else class="pb-favorites__layout">
      <ul class="pb-favorites__list" data-testid="favorites-list">
        <li
          v-for="favorite in favorites"
          :key="favorite.id"
          class="pb-favorites__entry"
          :class="{ 'pb-favorites__entry--active': selected?.id === favorite.id }"
          :data-testid="`favorite-${favorite.id}`"
        >
          <button type="button" class="pb-favorites__entry-button" @click="selectFavorite(favorite)">
            <span class="pb-favorites__name">{{ favorite.name }}</span>
            <span class="pb-favorites__meta">
              {{ t('prompt.favoriteItemCount', { count: favorite.item_count }) }} ·
              {{ favorite.visibility === 'public' ? t('prompt.favoritePublic') : t('prompt.favoritePrivate') }}
            </span>
          </button>
          <div v-if="viewingSelf && auth.isAuthenticated" class="pb-favorites__entry-actions">
            <el-button
              size="small"
              text
              :data-testid="`favorite-edit-${favorite.id}`"
              @click="editDialog(favorite)"
            >
              {{ t('me.save') }}
            </el-button>
            <el-button
              size="small"
              text
              type="danger"
              :data-testid="`favorite-delete-${favorite.id}`"
              @click="removeFavorite(favorite)"
            >
              {{ t('prompt.favoriteDelete') }}
            </el-button>
          </div>
        </li>
      </ul>

      <div class="pb-favorites__items">
        <p v-if="items.length === 0" data-testid="favorite-items-empty">
          {{ t('home.empty') }}
        </p>
        <ul v-else data-testid="favorite-items">
          <li
            v-for="item in items"
            :key="item.prompt_id"
            class="pb-favorites__item"
            :data-testid="`favorite-item-${item.prompt_id}`"
          >
            <RouterLink :to="`/prompts/${item.prompt_id}`" class="pb-favorites__item-title">
              {{ item.title }}
            </RouterLink>
            <span class="pb-favorites__item-meta">
              {{ item.version_no }} · {{ localized(item.purpose_name_zh, item.purpose_name_en) }}
              · {{ localized(item.capability_name_zh, item.capability_name_en) }}
            </span>
            <el-button
              v-if="viewingSelf && auth.isAuthenticated"
              size="small"
              text
              type="danger"
              :data-testid="`favorite-item-remove-${item.prompt_id}`"
              @click="removeItem(item)"
            >
              {{ t('prompt.favoriteRemove') }}
            </el-button>
          </li>
        </ul>

        <el-pagination
          v-if="itemsTotal > pageSize"
          layout="prev, pager, next"
          :current-page="itemsPage"
          :page-size="pageSize"
          :total="itemsTotal"
          @current-change="changeItemsPage"
        />
      </div>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="editing === null ? t('prompt.favoriteCreateHint') : t('prompt.favoriteDelete')"
      width="440px"
    >
      <el-form label-width="90px">
        <el-form-item :label="t('prompt.favoriteName')">
          <el-input v-model="form.name" maxlength="100" data-testid="favorite-name" />
        </el-form-item>
        <el-form-item :label="t('prompt.favoriteDescription')">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
            maxlength="500"
            data-testid="favorite-description"
          />
        </el-form-item>
        <el-form-item :label="t('prompt.favoriteVisibility')">
          <el-radio-group v-model="form.visibility" data-testid="favorite-visibility">
            <el-radio value="public">{{ t('prompt.favoritePublic') }}</el-radio>
            <el-radio value="private">{{ t('prompt.favoritePrivate') }}</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button data-testid="favorite-cancel" @click="dialogVisible = false">
          {{ t('auth.cancel') }}
        </el-button>
        <el-button type="primary" data-testid="favorite-submit" @click="submitForm">
          {{ t('me.save') }}
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.pb-favorites {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-favorites__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-favorites__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
}

.pb-favorites__title {
  font-size: 30px;
  font-weight: 600;
  letter-spacing: 0.01em;
  margin: 0;
}

.pb-favorites__layout {
  display: grid;
  grid-template-columns: minmax(220px, 280px) minmax(0, 1fr);
  gap: var(--pb-space-lg);
  align-items: start;
}

.pb-favorites__list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  list-style: none;
  margin: 0;
  padding: var(--pb-space-sm);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
}

.pb-favorites__entry {
  padding: var(--pb-space-sm) var(--pb-space-sm) var(--pb-space-sm) var(--pb-space-md);
  border-left: 2px solid transparent;
  border-radius: var(--pb-radius-sm);
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease);
}

.pb-favorites__entry:hover {
  background: var(--pb-surface-muted);
}

.pb-favorites__entry--active {
  border-left-color: var(--pb-accent);
  background: var(--pb-accent-soft);
}

.pb-favorites__entry-button {
  width: 100%;
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
}

.pb-favorites__name {
  display: block;
  color: var(--pb-ink);
  font-size: 14px;
  font-weight: 600;
}

.pb-favorites__meta,
.pb-favorites__item-meta {
  display: block;
  margin-top: 2px;
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-favorites__entry-actions {
  display: flex;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-xs);
}

.pb-favorites__items {
  min-width: 0;
  padding: var(--pb-space-lg);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
}

.pb-favorites__items ul {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-favorites__item {
  display: flex;
  align-items: center;
  gap: var(--pb-space-sm);
  padding: var(--pb-space-md) 0;
  border-bottom: 1px solid var(--pb-line);
}

.pb-favorites__item:last-child {
  border-bottom: 0;
}

.pb-favorites__item-title {
  flex: 1;
  min-width: 0;
  color: var(--pb-ink);
  font-size: 15px;
  font-weight: 500;
  overflow-wrap: anywhere;
}

.pb-favorites__item-title:hover {
  color: var(--pb-accent);
}

.pb-favorites__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

.pb-favorites__notice,
.pb-favorites__hint {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
}

@media (max-width: 720px) {
  .pb-favorites__layout {
    grid-template-columns: 1fr;
  }
}
</style>