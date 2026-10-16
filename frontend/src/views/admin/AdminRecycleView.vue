// 回收站管理（需求 §11.3、§2.4）。
// 只有提示词条目可以恢复与彻底删除；库与库内问题由所属用户自行处理。

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type AdminRecycleEntry, adminApi, type RecycleObjectType } from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()
const { messageOf } = useErrorMessage()

const rows = ref<AdminRecycleEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const failure = ref('')

const filters = ref({ objectType: '' })

/** typeLabel 返回对象类型的中文文案。 */
function typeLabel(objectType: RecycleObjectType): string {
  if (objectType === 'prompt') {
    return t('admin.recycle.typePrompt')
  }
  if (objectType === 'library') {
    return t('admin.recycle.typeLibrary')
  }
  return t('admin.recycle.typeLibraryQuestion')
}

async function loadEntries(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.recycleBin({
      page: page.value,
      page_size: pageSize,
      object_type: filters.value.objectType,
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
  void loadEntries()
}

function resetFilters(): void {
  filters.value = { objectType: '' }
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadEntries()
}

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? raw : parsed.toLocaleString()
}

async function restoreEntry(entry: AdminRecycleEntry): Promise<void> {
  try {
    await adminApi.restoreRecycleEntry(entry.object_type, entry.object_id)
    ElMessage.success(t('admin.recycle.restored'))
    await loadEntries()
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

async function purgeEntry(entry: AdminRecycleEntry): Promise<void> {
  try {
    await adminApi.purgeRecycleEntry(entry.object_type, entry.object_id)
    ElMessage.success(t('admin.recycle.purged'))
    await loadEntries()
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

onMounted(async () => {
  await loadEntries()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title" data-testid="admin-recycle-title">
        {{ t('admin.recycle.title') }}
      </h2>
    </header>

    <p class="pb-admin-module__hint">{{ t('admin.recycle.libraryReadOnly') }}</p>
    <p class="pb-admin-module__hint">{{ t('admin.recycle.purgeHint') }}</p>

    <div class="pb-admin-module__filters">
      <el-select
        v-model="filters.objectType"
        class="pb-admin-module__filter"
        :placeholder="t('admin.recycle.objectType')"
        clearable
        data-testid="admin-recycle-type"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('admin.recycle.typePrompt')" value="prompt" />
        <el-option :label="t('admin.recycle.typeLibrary')" value="library" />
        <el-option :label="t('admin.recycle.typeLibraryQuestion')" value="library_question" />
      </el-select>
      <el-button type="primary" size="small" data-testid="admin-recycle-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-recycle-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-recycle-error">
      {{ t('admin.common.loadFailed') }}：{{ failure }}
    </p>
    <p
      v-if="!loading && rows.length === 0"
      class="pb-admin-module__empty"
      data-testid="admin-recycle-empty"
    >
      {{ t('admin.recycle.empty') }}
    </p>

    <el-table v-if="rows.length > 0" :data="rows" size="small" data-testid="admin-recycle-table">
      <el-table-column :label="t('admin.recycle.objectType')" width="130">
        <template #default="{ row }: { row: AdminRecycleEntry }">
          {{ typeLabel(row.object_type) }}
        </template>
      </el-table-column>
      <el-table-column prop="title" :label="t('admin.recycle.objectTitle')" min-width="200" />
      <el-table-column prop="owner_name" :label="t('admin.recycle.owner')" width="140" />
      <el-table-column prop="deleted_by_name" :label="t('admin.recycle.deletedBy')" width="140" />
      <el-table-column :label="t('admin.recycle.deletedAt')" width="180">
        <template #default="{ row }: { row: AdminRecycleEntry }">
          {{ formatTime(row.deleted_at) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.recycle.expiresAt')" width="180">
        <template #default="{ row }: { row: AdminRecycleEntry }">
          {{ formatTime(row.expires_at) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.common.actions')" width="190">
        <template #default="{ row }: { row: AdminRecycleEntry }">
          <el-button
            size="small"
            text
            :disabled="row.object_type !== 'prompt'"
            data-testid="admin-recycle-restore"
            @click="restoreEntry(row)"
          >
            {{ t('admin.recycle.restore') }}
          </el-button>
          <el-popconfirm
            width="260"
            :title="t('admin.common.confirmPurge')"
            :confirm-button-text="t('admin.common.confirm')"
            :cancel-button-text="t('admin.common.cancel')"
            @confirm="purgeEntry(row)"
          >
            <template #reference>
              <el-button
                size="small"
                text
                type="danger"
                :disabled="row.object_type !== 'prompt'"
                data-testid="admin-recycle-purge"
              >
                {{ t('admin.recycle.purge') }}
              </el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-if="total > 0"
      class="pb-admin-module__pager"
      layout="prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="changePage"
    />
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
</style>
