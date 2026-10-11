<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  type PromptPayload,
  promptApi,
  type SensitiveLevel,
  type Visibility,
  visibilityOptions,
} from '@/api/prompt'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { useDictionaryStore } from '@/stores/dictionary'
import { normalizeVisibility, visibleCategories } from '@/views/promptFormRules'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const auth = useAuthStore()
const dictionary = useDictionaryStore()
const { messageOf } = useErrorMessage()

const promptID = computed(() => {
  const raw = route.params.id
  const parsed = Number.parseInt(String(raw ?? ''), 10)
  return Number.isNaN(parsed) ? null : parsed
})
const isEdit = computed(() => promptID.value !== null)

interface Form {
  title: string
  body: string
  description: string
  categoryID: number | undefined
  abilityID: number | undefined
  difficultyID: number | undefined
  visibility: Visibility
  sensitiveLevel: SensitiveLevel
  tagIDs: number[]
  changeNote: string
  isMajorChange: boolean
}

function emptyForm(): Form {
  return {
    title: '',
    body: '',
    description: '',
    categoryID: undefined,
    abilityID: undefined,
    difficultyID: undefined,
    visibility: 'public',
    sensitiveLevel: 'low',
    tagIDs: [],
    changeNote: '',
    isMajorChange: false,
  }
}

const form = ref<Form>(emptyForm())
const submitting = ref(false)
const notice = ref('')
const failure = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

function localized(zh: string, en: string): string {
  return locale.value === 'en' ? en : zh
}

/**
 * 可选可见性档位随身份收敛。「开发者可见」只对开发者与管理员出现，
 * 与后端的判定保持同一来源：普通用户既看不到也无法通过接口选中。
 */
const visibilities = computed(() => visibilityOptions(auth.hasRole('developer')))

const sensitiveLevels = computed(() => [
  { value: 'low' as const, label: t('prompt.sensitiveLow') },
  { value: 'medium' as const, label: t('prompt.sensitiveMedium') },
  { value: 'high' as const, label: t('prompt.sensitiveHigh') },
])

/** 高风险类别不出现在普通用户的表单中，与后端 C-19 的判定同源。 */
const categoryOptions = computed(() =>
  visibleCategories(dictionary.data.categories, auth.hasRole('developer')),
)

function openFilePicker(): void {
  fileInput.value?.click()
}

async function onFilePicked(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (file === undefined) {
    return
  }

  failure.value = ''
  try {
    const parsed = await promptApi.importMarkdown(file)
    form.value.title = parsed.title
    form.value.body = parsed.body
    if (parsed.description !== '') {
      form.value.description = parsed.description
    }
    notice.value = t('prompt.importMarkdown')
  } catch (error) {
    failure.value = `${t('prompt.importFailed')} ${messageOf(error)}`
  }
}

function payload(): PromptPayload {
  return {
    title: form.value.title.trim(),
    body: form.value.body,
    description: form.value.description.trim(),
    category_id: form.value.categoryID ?? 0,
    ability_id: form.value.abilityID,
    difficulty_id: form.value.difficultyID,
    visibility: normalizeVisibility(form.value.visibility, auth.hasRole('developer')),
    sensitive_level: form.value.sensitiveLevel,
    tag_ids: form.value.tagIDs,
    change_note: form.value.changeNote.trim(),
    is_major_change: form.value.isMajorChange,
  }
}

async function submit(): Promise<void> {
  submitting.value = true
  notice.value = ''
  failure.value = ''
  try {
    const result =
      promptID.value === null
        ? await promptApi.create(payload())
        : await promptApi.update(promptID.value, payload())
    notice.value = result.major_change
      ? t('prompt.updatedMajor')
      : isEdit.value
        ? t('prompt.updated')
        : t('prompt.created')
    await router.push(`/prompts/${result.prompt.id}`)
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await Promise.all([dictionary.load(), auth.connect()])
  if (promptID.value === null) {
    return
  }
  try {
    const detail = await promptApi.detail(promptID.value)
    form.value = {
      title: detail.version.title,
      body: detail.version.body,
      description: detail.version.description,
      categoryID: detail.prompt.category_id,
      abilityID: detail.prompt.ability_id,
      difficultyID: detail.prompt.difficulty_id,
      visibility: detail.prompt.visibility,
      sensitiveLevel: detail.prompt.sensitive_level,
      tagIDs: detail.tags.map((tag) => tag.id),
      changeNote: '',
      isMajorChange: false,
    }
  } catch (error) {
    failure.value = messageOf(error)
  }
})
</script>

<template>
  <section class="pb-prompt-form">
    <h1 class="pb-prompt-form__title">
      {{ isEdit ? t('prompt.editTitle') : t('prompt.uploadTitle') }}
    </h1>

    <el-form data-testid="prompt-form" label-position="top" @submit.prevent="submit">
      <el-form-item :label="t('prompt.title')">
        <el-input v-model="form.title" data-testid="prompt-title" maxlength="200" show-word-limit />
      </el-form-item>

      <el-form-item :label="t('prompt.body')">
        <el-input
          v-model="form.body"
          data-testid="prompt-body"
          type="textarea"
          :rows="12"
          maxlength="200000"
          show-word-limit
        />
      </el-form-item>

      <el-form-item :label="t('prompt.description')">
        <el-input
          v-model="form.description"
          data-testid="prompt-description"
          type="textarea"
          :rows="3"
          maxlength="2000"
        />
      </el-form-item>

      <div class="pb-prompt-form__row">
        <el-form-item :label="t('prompt.category')">
          <el-select v-model="form.categoryID" data-testid="prompt-category" :placeholder="t('prompt.category')">
            <el-option
              v-for="item in categoryOptions"
              :key="item.id"
              :label="localized(item.name_zh, item.name_en)"
              :value="item.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('prompt.ability')">
          <el-select v-model="form.abilityID" data-testid="prompt-ability" clearable :placeholder="t('prompt.ability')">
            <el-option
              v-for="item in dictionary.data.abilities"
              :key="item.id"
              :label="localized(item.name_zh, item.name_en)"
              :value="item.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('prompt.difficulty')">
          <el-select v-model="form.difficultyID" data-testid="prompt-difficulty" clearable :placeholder="t('prompt.difficulty')">
            <el-option
              v-for="item in dictionary.data.difficulties"
              :key="item.id"
              :label="localized(item.name_zh, item.name_en)"
              :value="item.id"
            />
          </el-select>
        </el-form-item>
      </div>

      <div class="pb-prompt-form__row">
        <el-form-item :label="t('prompt.visibility')">
          <el-select v-model="form.visibility" data-testid="prompt-visibility">
            <el-option
              v-for="item in visibilities"
              :key="item"
              :label="t(`prompt.visibility${item.charAt(0).toUpperCase()}${item.slice(1)}`)"
              :value="item"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('prompt.sensitiveLevel')">
          <el-select v-model="form.sensitiveLevel" data-testid="prompt-sensitive">
            <el-option
              v-for="item in sensitiveLevels"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('prompt.tags')">
          <el-select
            v-model="form.tagIDs"
            data-testid="prompt-tags"
            multiple
            collapse-tags
            :placeholder="t('prompt.tags')"
          >
            <el-option
              v-for="item in dictionary.data.tags"
              :key="item.id"
              :label="localized(item.name_zh, item.name_en)"
              :value="item.id"
            />
          </el-select>
        </el-form-item>
      </div>

      <template v-if="isEdit">
        <el-form-item :label="t('prompt.changeNote')">
          <el-input v-model="form.changeNote" data-testid="prompt-change-note" maxlength="500" />
        </el-form-item>

        <el-form-item>
          <el-checkbox v-model="form.isMajorChange" data-testid="prompt-major">
            {{ t('prompt.majorChange') }}
          </el-checkbox>
          <p v-if="form.isMajorChange" class="pb-prompt-form__hint">{{ t('prompt.majorChangeHint') }}</p>
        </el-form-item>
      </template>

      <div class="pb-prompt-form__actions">
        <el-button
          data-testid="prompt-import"
          @click="openFilePicker"
        >
          {{ t('prompt.importMarkdown') }}
        </el-button>
        <input
          ref="fileInput"
          class="pb-prompt-form__file"
          type="file"
          accept=".md"
          data-testid="prompt-file"
          @change="onFilePicked"
        />

        <el-button
          type="primary"
          data-testid="prompt-submit"
          :loading="submitting"
          @click="submit"
        >
          {{ isEdit ? t('prompt.submitUpdate') : t('prompt.submitCreate') }}
        </el-button>
      </div>

      <p v-if="notice" class="pb-prompt-form__notice" data-testid="prompt-notice">{{ notice }}</p>
      <p v-if="failure" class="pb-prompt-form__failure" data-testid="prompt-error">{{ failure }}</p>
    </el-form>
  </section>
</template>

<style scoped>
.pb-prompt-form {
  padding: var(--pb-space-lg) 0;
  max-width: 900px;
}

.pb-prompt-form__title {
  font-size: 24px;
  margin: 0 0 var(--pb-space-md);
}

.pb-prompt-form__row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--pb-space-md);
}

.pb-prompt-form__row :deep(.el-select) {
  width: 100%;
}

.pb-prompt-form__file {
  display: none;
}

.pb-prompt-form__hint {
  color: var(--el-color-warning);
  font-size: 13px;
  margin: var(--pb-space-xs) 0 0;
}

.pb-prompt-form__actions {
  display: flex;
  gap: var(--pb-space-sm);
  margin-top: var(--pb-space-md);
}

.pb-prompt-form__notice {
  color: var(--el-color-success);
  margin: var(--pb-space-sm) 0 0;
}

.pb-prompt-form__failure {
  color: var(--el-color-danger);
  margin: var(--pb-space-sm) 0 0;
}
</style>