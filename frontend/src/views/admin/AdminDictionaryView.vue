// 分类字典管理：维护考察维度与考察目的。被引用的条目只能停用，不能删除。

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type AdminCapability,
  type AdminCapabilityInput,
  type AdminPurpose,
  type AdminPurposeInput,
  adminApi,
} from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'

type TabName = 'capabilities' | 'purposes'

const { t, te, locale } = useI18n()
const { messageOf } = useErrorMessage()

const activeTab = ref<TabName>('capabilities')
const keyword = ref('')
const loading = ref(false)
const failure = ref('')

const capabilities = ref<AdminCapability[]>([])
const purposes = ref<AdminPurpose[]>([])

const capabilityDialog = ref(false)
const capabilityEditingID = ref<number | null>(null)
const capabilityForm = ref<AdminCapabilityInput>(emptyCapability())
const capabilitySaving = ref(false)
const capabilityFailure = ref('')

const purposeDialog = ref(false)
const purposeEditingID = ref<number | null>(null)
const purposeForm = ref<AdminPurposeInput>(emptyPurpose())
const purposeSaving = ref(false)
const purposeFailure = ref('')

function emptyCapability(): AdminCapabilityInput {
  return {
    slug: '',
    name_zh: '',
    name_en: '',
    group_key: 'cognitive',
    description_zh: '',
    description_en: '',
    is_enabled: true,
    sort_order: 0,
  }
}

function emptyPurpose(): AdminPurposeInput {
  return {
    slug: '',
    name_zh: '',
    name_en: '',
    description_zh: '',
    description_en: '',
    is_high_risk: false,
    is_enabled: true,
    sort_order: 0,
  }
}

function localizedName(nameZh: string, nameEn: string): string {
  return locale.value === 'en' ? nameEn : nameZh
}

function localizedDescription(zh: string, en: string): string {
  return locale.value === 'en' ? en : zh
}

function groupLabel(groupKey: string): string {
  const key = `admin.dictionary.group${groupKey.charAt(0).toUpperCase()}${groupKey.slice(1)}`
  return te(key) ? t(key) : groupKey
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const [capabilityPage, purposePage] = await Promise.all([
      adminApi.capabilities({ page: 1, page_size: 100, keyword: keyword.value }),
      adminApi.purposes({ page: 1, page_size: 100, keyword: keyword.value }),
    ])
    capabilities.value = capabilityPage.items
    purposes.value = purposePage.items
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

function search(): void {
  void load()
}

function resetFilters(): void {
  keyword.value = ''
  void load()
}

function openCapabilityCreate(): void {
  capabilityEditingID.value = null
  capabilityForm.value = emptyCapability()
  capabilityForm.value.sort_order = capabilities.value.length + 1
  capabilityFailure.value = ''
  capabilityDialog.value = true
}

function openCapabilityEdit(row: AdminCapability): void {
  capabilityEditingID.value = row.id
  capabilityForm.value = {
    slug: row.slug,
    name_zh: row.name_zh,
    name_en: row.name_en,
    group_key: row.group_key,
    description_zh: row.description_zh,
    description_en: row.description_en,
    is_enabled: row.is_enabled,
    sort_order: row.sort_order,
  }
  capabilityFailure.value = ''
  capabilityDialog.value = true
}

async function saveCapability(): Promise<void> {
  capabilitySaving.value = true
  try {
    if (capabilityEditingID.value === null) {
      await adminApi.createCapability(capabilityForm.value)
      ElMessage.success(t('admin.dictionary.capabilityCreated'))
    } else {
      await adminApi.updateCapability(capabilityEditingID.value, capabilityForm.value)
      ElMessage.success(t('admin.dictionary.capabilityUpdated'))
    }
    capabilityDialog.value = false
    await load()
  } catch (error) {
    capabilityFailure.value = messageOf(error)
  } finally {
    capabilitySaving.value = false
  }
}

async function removeCapability(row: AdminCapability): Promise<void> {
  try {
    await ElMessageBox.confirm(
      t('admin.dictionary.deleteCapabilityHint'),
      t('admin.common.confirmDelete'),
      { type: 'warning' },
    )
    await adminApi.deleteCapability(row.id)
    ElMessage.success(t('admin.dictionary.capabilityDeleted'))
    await load()
  } catch (error) {
    if (error === 'cancel' || error === 'close') {
      return
    }
    failure.value = messageOf(error)
  }
}

function openPurposeCreate(): void {
  purposeEditingID.value = null
  purposeForm.value = emptyPurpose()
  purposeForm.value.sort_order = purposes.value.length + 1
  purposeFailure.value = ''
  purposeDialog.value = true
}

function openPurposeEdit(row: AdminPurpose): void {
  purposeEditingID.value = row.id
  purposeForm.value = {
    slug: row.slug,
    name_zh: row.name_zh,
    name_en: row.name_en,
    description_zh: row.description_zh,
    description_en: row.description_en,
    is_high_risk: row.is_high_risk,
    is_enabled: row.is_enabled,
    sort_order: row.sort_order,
  }
  purposeFailure.value = ''
  purposeDialog.value = true
}

async function savePurpose(): Promise<void> {
  purposeSaving.value = true
  try {
    if (purposeEditingID.value === null) {
      await adminApi.createPurpose(purposeForm.value)
      ElMessage.success(t('admin.dictionary.purposeCreated'))
    } else {
      await adminApi.updatePurpose(purposeEditingID.value, purposeForm.value)
      ElMessage.success(t('admin.dictionary.purposeUpdated'))
    }
    purposeDialog.value = false
    await load()
  } catch (error) {
    purposeFailure.value = messageOf(error)
  } finally {
    purposeSaving.value = false
  }
}

async function removePurpose(row: AdminPurpose): Promise<void> {
  try {
    await ElMessageBox.confirm(
      t('admin.dictionary.deletePurposeHint'),
      t('admin.common.confirmDelete'),
      { type: 'warning' },
    )
    await adminApi.deletePurpose(row.id)
    ElMessage.success(t('admin.dictionary.purposeDeleted'))
    await load()
  } catch (error) {
    if (error === 'cancel' || error === 'close') {
      return
    }
    failure.value = messageOf(error)
  }
}

onMounted(load)
</script>

<template>
  <section class="pb-admin-module" data-testid="admin-dictionary">
    <header class="pb-admin-module__head">
      <div>
        <h2 class="pb-admin-module__title">{{ t('admin.dictionary.title') }}</h2>
        <p class="pb-admin-module__subtitle">{{ t('admin.dictionary.subtitle') }}</p>
      </div>
    </header>

    <p v-if="failure" class="pb-admin-module__error">{{ failure }}</p>

    <div class="pb-admin-module__filters">
      <el-input
        v-model="keyword"
        class="pb-admin-module__filter"
        :placeholder="t('admin.dictionary.keyword')"
        clearable
        data-testid="admin-dictionary-keyword"
        @keyup.enter="search"
      />
      <el-button type="primary" size="small" data-testid="admin-dictionary-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-dictionary-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <el-tabs v-model="activeTab" data-testid="admin-dictionary-tabs">
      <el-tab-pane :label="t('admin.dictionary.capabilities')" name="capabilities">
        <div class="pb-admin-module__toolbar">
          <el-button
            type="primary"
            size="small"
            data-testid="admin-dictionary-create-capability"
            @click="openCapabilityCreate"
          >
            {{ t('admin.dictionary.createCapability') }}
          </el-button>
        </div>
        <el-table
          v-loading="loading"
          :data="capabilities"
          size="small"
          data-testid="admin-dictionary-capability-table"
        >
          <el-table-column :label="t('admin.dictionary.name')" min-width="150">
            <template #default="{ row }: { row: AdminCapability }">
              {{ localizedName(row.name_zh, row.name_en) }}
            </template>
          </el-table-column>
          <el-table-column prop="slug" :label="t('admin.dictionary.slug')" width="170" />
          <el-table-column :label="t('admin.dictionary.group')" width="110">
            <template #default="{ row }: { row: AdminCapability }">
              {{ groupLabel(row.group_key) }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.dictionary.description')" min-width="220">
            <template #default="{ row }: { row: AdminCapability }">
              {{ localizedDescription(row.description_zh, row.description_en) }}
            </template>
          </el-table-column>
          <el-table-column prop="sort_order" :label="t('admin.dictionary.sortOrder')" width="90" />
          <el-table-column :label="t('admin.common.enabled')" width="100">
            <template #default="{ row }: { row: AdminCapability }">
              {{ row.is_enabled ? t('admin.common.enabled') : t('admin.common.disabled') }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.common.actions')" width="150">
            <template #default="{ row }: { row: AdminCapability }">
              <el-button size="small" text @click="openCapabilityEdit(row)">
                {{ t('admin.common.edit') }}
              </el-button>
              <el-button size="small" text type="danger" @click="removeCapability(row)">
                {{ t('admin.common.remove') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane :label="t('admin.dictionary.purposes')" name="purposes">
        <div class="pb-admin-module__toolbar">
          <el-button
            type="primary"
            size="small"
            data-testid="admin-dictionary-create-purpose"
            @click="openPurposeCreate"
          >
            {{ t('admin.dictionary.createPurpose') }}
          </el-button>
        </div>
        <el-table
          v-loading="loading"
          :data="purposes"
          size="small"
          data-testid="admin-dictionary-purpose-table"
        >
          <el-table-column :label="t('admin.dictionary.name')" min-width="150">
            <template #default="{ row }: { row: AdminPurpose }">
              {{ localizedName(row.name_zh, row.name_en) }}
            </template>
          </el-table-column>
          <el-table-column prop="slug" :label="t('admin.dictionary.slug')" width="190" />
          <el-table-column :label="t('admin.dictionary.description')" min-width="240">
            <template #default="{ row }: { row: AdminPurpose }">
              {{ localizedDescription(row.description_zh, row.description_en) }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.dictionary.highRisk')" width="100">
            <template #default="{ row }: { row: AdminPurpose }">
              {{ row.is_high_risk ? t('admin.common.yes') : t('admin.common.no') }}
            </template>
          </el-table-column>
          <el-table-column prop="sort_order" :label="t('admin.dictionary.sortOrder')" width="90" />
          <el-table-column :label="t('admin.common.enabled')" width="100">
            <template #default="{ row }: { row: AdminPurpose }">
              {{ row.is_enabled ? t('admin.common.enabled') : t('admin.common.disabled') }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.common.actions')" width="150">
            <template #default="{ row }: { row: AdminPurpose }">
              <el-button size="small" text @click="openPurposeEdit(row)">
                {{ t('admin.common.edit') }}
              </el-button>
              <el-button size="small" text type="danger" @click="removePurpose(row)">
                {{ t('admin.common.remove') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      v-model="capabilityDialog"
      :title="
        capabilityEditingID === null
          ? t('admin.dictionary.createCapability')
          : t('admin.dictionary.editCapability')
      "
      width="620px"
    >
      <p v-if="capabilityFailure" class="pb-admin-module__error">{{ capabilityFailure }}</p>
      <el-form label-width="120px">
        <el-form-item :label="t('admin.dictionary.slug')">
          <el-input v-model="capabilityForm.slug" :disabled="capabilityEditingID !== null" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.nameZh')">
          <el-input v-model="capabilityForm.name_zh" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.nameEn')">
          <el-input v-model="capabilityForm.name_en" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.group')">
          <el-select v-model="capabilityForm.group_key">
            <el-option :label="t('admin.dictionary.groupCognitive')" value="cognitive" />
            <el-option :label="t('admin.dictionary.groupExecution')" value="execution" />
            <el-option :label="t('admin.dictionary.groupReliability')" value="reliability" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.descriptionZh')">
          <el-input v-model="capabilityForm.description_zh" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.descriptionEn')">
          <el-input v-model="capabilityForm.description_en" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.sortOrder')">
          <el-input-number v-model="capabilityForm.sort_order" :min="-1000" :max="1000" />
        </el-form-item>
        <el-form-item :label="t('admin.common.enabled')">
          <el-switch v-model="capabilityForm.is_enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="capabilityDialog = false">{{ t('admin.common.cancel') }}</el-button>
        <el-button type="primary" :loading="capabilitySaving" @click="saveCapability">
          {{ t('admin.common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="purposeDialog"
      :title="
        purposeEditingID === null
          ? t('admin.dictionary.createPurpose')
          : t('admin.dictionary.editPurpose')
      "
      width="620px"
    >
      <p v-if="purposeFailure" class="pb-admin-module__error">{{ purposeFailure }}</p>
      <el-form label-width="120px">
        <el-form-item :label="t('admin.dictionary.slug')">
          <el-input v-model="purposeForm.slug" :disabled="purposeEditingID !== null" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.nameZh')">
          <el-input v-model="purposeForm.name_zh" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.nameEn')">
          <el-input v-model="purposeForm.name_en" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.descriptionZh')">
          <el-input v-model="purposeForm.description_zh" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.descriptionEn')">
          <el-input v-model="purposeForm.description_en" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.highRisk')">
          <el-switch v-model="purposeForm.is_high_risk" />
        </el-form-item>
        <el-form-item :label="t('admin.dictionary.sortOrder')">
          <el-input-number v-model="purposeForm.sort_order" :min="-1000" :max="1000" />
        </el-form-item>
        <el-form-item :label="t('admin.common.enabled')">
          <el-switch v-model="purposeForm.is_enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="purposeDialog = false">{{ t('admin.common.cancel') }}</el-button>
        <el-button type="primary" :loading="purposeSaving" @click="savePurpose">
          {{ t('admin.common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>
