<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type AdminShowcaseImage, type AdminShowcaseOutput, adminApi } from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { formatDateTime } from '@/utils/datetime'

const { t, locale } = useI18n()
const { messageOf } = useErrorMessage()

type Tab = 'images' | 'outputs'

const tab = ref<Tab>('images')
const images = ref<AdminShowcaseImage[]>([])
const outputs = ref<AdminShowcaseOutput[]>([])
const imageTotal = ref(0)
const outputTotal = ref(0)
const imagePage = ref(1)
const outputPage = ref(1)
const pageSize = 20
const loading = ref(false)
const failure = ref('')

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return t('admin.common.none')
  }
  return formatDateTime(raw, locale.value)
}

/** truncate 把长文本压成单行摘要。 */
function truncate(text: string, limit: number): string {
  return text.length <= limit ? text : `${text.slice(0, limit)}…`
}

async function loadImages(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.showcaseImages({ page: imagePage.value, page_size: pageSize })
    images.value = result.items
    imageTotal.value = result.total
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

async function loadOutputs(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.showcaseOutputs({ page: outputPage.value, page_size: pageSize })
    outputs.value = result.items
    outputTotal.value = result.total
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

async function loadCurrent(): Promise<void> {
  if (tab.value === 'images') {
    await loadImages()
  } else {
    await loadOutputs()
  }
}

function switchTab(name: string): void {
  tab.value = name as Tab
  void loadCurrent()
}

function changeImagePage(next: number): void {
  imagePage.value = next
  void loadImages()
}

function changeOutputPage(next: number): void {
  outputPage.value = next
  void loadOutputs()
}

/** confirmDelete 要求管理员二次确认；违规判定由管理员完成。 */
async function confirmDelete(kind: Tab, id: number): Promise<void> {
  try {
    await ElMessageBox.confirm(t('admin.showcase.deleteConfirm'), t('admin.showcase.delete'), {
      confirmButtonText: t('admin.common.confirm'),
      cancelButtonText: t('admin.common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    if (kind === 'images') {
      await adminApi.deleteShowcaseImage(id)
    } else {
      await adminApi.deleteShowcaseOutput(id)
    }
    ElMessage.success(t('admin.showcase.deleted'))
    await loadCurrent()
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

onMounted(loadImages)
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title">{{ t('admin.showcase.title') }}</h2>
    </header>

    <p class="pb-admin-module__hint">{{ t('admin.showcase.hint') }}</p>

    <el-tabs :model-value="tab" data-testid="admin-showcase-tabs" @tab-change="switchTab">
      <el-tab-pane :label="t('admin.showcase.tabImages')" name="images">
        <p v-if="failure" class="pb-admin-module__error" data-testid="admin-showcase-error">
          {{ failure }}
        </p>
        <p
          v-if="!loading && images.length === 0"
          class="pb-admin-module__empty"
          data-testid="admin-showcase-empty"
        >
          {{ t('admin.common.empty') }}
        </p>

        <el-table
          v-if="images.length > 0"
          :data="images"
          size="small"
          data-testid="admin-showcase-image-table"
        >
          <el-table-column :label="t('admin.showcase.preview')" width="120">
            <template #default="{ row }: { row: AdminShowcaseImage }">
              <el-image
                :src="row.url"
                :alt="row.filename"
                :preview-src-list="[row.url]"
                preview-teleported
                fit="cover"
                class="pb-admin-showcase__thumb"
              />
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.showcase.prompt')" min-width="200">
            <template #default="{ row }: { row: AdminShowcaseImage }">
              <RouterLink class="pb-admin-showcase__link" :to="`/prompts/${row.prompt_id}`">
                {{ truncate(row.prompt_title, 40) }}
              </RouterLink>
            </template>
          </el-table-column>
          <el-table-column
            prop="model_name"
            :label="t('admin.showcase.model')"
            width="170"
          />
          <el-table-column
            prop="uploader"
            :label="t('admin.showcase.uploader')"
            width="130"
          />
          <el-table-column :label="t('admin.showcase.uploadedAt')" width="180">
            <template #default="{ row }: { row: AdminShowcaseImage }">
              {{ formatTime(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.common.actions')" width="110">
            <template #default="{ row }: { row: AdminShowcaseImage }">
              <el-button
                size="small"
                text
                type="danger"
                :data-testid="`admin-showcase-image-delete-${row.id}`"
                @click="confirmDelete('images', row.id)"
              >
                {{ t('admin.showcase.delete') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-pagination
          class="pb-admin-module__pager"
          layout="prev, pager, next"
          :current-page="imagePage"
          :page-size="pageSize"
          :total="imageTotal"
          @current-change="changeImagePage"
        />
      </el-tab-pane>

      <el-tab-pane :label="t('admin.showcase.tabOutputs')" name="outputs">
        <p
          v-if="!loading && outputs.length === 0"
          class="pb-admin-module__empty"
          data-testid="admin-showcase-output-empty"
        >
          {{ t('admin.common.empty') }}
        </p>

        <el-table
          v-if="outputs.length > 0"
          :data="outputs"
          size="small"
          data-testid="admin-showcase-output-table"
        >
          <el-table-column :label="t('admin.showcase.content')" min-width="300">
            <template #default="{ row }: { row: AdminShowcaseOutput }">
              <span class="pb-admin-showcase__content">{{ truncate(row.content, 120) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.showcase.prompt')" min-width="180">
            <template #default="{ row }: { row: AdminShowcaseOutput }">
              <RouterLink class="pb-admin-showcase__link" :to="`/prompts/${row.prompt_id}`">
                {{ truncate(row.prompt_title, 30) }}
              </RouterLink>
            </template>
          </el-table-column>
          <el-table-column
            prop="model_name"
            :label="t('admin.showcase.model')"
            width="160"
          />
          <el-table-column
            prop="uploader"
            :label="t('admin.showcase.uploader')"
            width="120"
          />
          <el-table-column :label="t('admin.showcase.uploadedAt')" width="170">
            <template #default="{ row }: { row: AdminShowcaseOutput }">
              {{ formatTime(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.common.actions')" width="100">
            <template #default="{ row }: { row: AdminShowcaseOutput }">
              <el-button
                size="small"
                text
                type="danger"
                :data-testid="`admin-showcase-output-delete-${row.id}`"
                @click="confirmDelete('outputs', row.id)"
              >
                {{ t('admin.showcase.delete') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-pagination
          class="pb-admin-module__pager"
          layout="prev, pager, next"
          :current-page="outputPage"
          :page-size="pageSize"
          :total="outputTotal"
          @current-change="changeOutputPage"
        />
      </el-tab-pane>
    </el-tabs>
  </section>
</template>

<style scoped>
.pb-admin-showcase__thumb {
  width: 88px;
  height: 66px;
  border-radius: var(--pb-radius-sm);
  border: 1px solid var(--pb-line);
}

.pb-admin-showcase__link {
  color: var(--pb-accent-strong);
  border-bottom: 1px solid var(--pb-accent-ring);
}

.pb-admin-showcase__content {
  color: var(--pb-ink-soft);
  font-family: var(--pb-font-mono);
  font-size: 11.5px;
  line-height: 1.6;
}
</style>
