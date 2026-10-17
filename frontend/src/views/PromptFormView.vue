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
import { normalizeVisibility, visiblePurposes } from '@/views/promptFormRules'

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
  purposeID: number | undefined
  capabilityID: number | undefined
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
    purposeID: undefined,
    capabilityID: undefined,
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
/**
 * loaded 标记编辑表单已取回待修改的提示词。
 *
 * 表单在详情返回之前就已经渲染，此时提交会把占位取值（空正文、考察目的 0）发出去，
 * 后端必然以字段校验失败拒绝。提交入口在详情到达前保持禁用，避免这种必然失败的请求。
 */
const loaded = ref(false)
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

/** 高风险考察目的不出现在普通用户的表单中，与后端 C-19 的判定同源。 */
const purposeOptions = computed(() =>
  visiblePurposes(dictionary.data.purposes, auth.hasRole('developer')),
)

const tagGroups = computed(() =>
  [
    {
      label: t('prompt.themeTags'),
      options: dictionary.data.tags.filter((tag) => tag.kind === 'theme'),
    },
    {
      label: t('prompt.formTags'),
      options: dictionary.data.tags.filter((tag) => tag.kind === 'form'),
    },
  ].filter((group) => group.options.length > 0),
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
    purpose_id: form.value.purposeID ?? 0,
    capability_id: form.value.capabilityID ?? 0,
    difficulty_id: form.value.difficultyID,
    visibility: normalizeVisibility(form.value.visibility, auth.hasRole('developer')),
    sensitive_level: form.value.sensitiveLevel,
    tag_ids: form.value.tagIDs,
    change_note: form.value.changeNote.trim(),
    is_major_change: form.value.isMajorChange,
  }
}

async function submit(): Promise<void> {
  // 编辑模式下详情未到达时表单里是占位取值，提交必然被后端拒绝。
  if (isEdit.value && !loaded.value) {
    return
  }
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
      purposeID: detail.prompt.purpose_id,
      capabilityID: detail.prompt.capability_id,
      difficultyID: detail.prompt.difficulty_id,
      visibility: detail.prompt.visibility,
      sensitiveLevel: detail.prompt.sensitive_level,
      tagIDs: detail.tags.map((tag) => tag.id),
      changeNote: '',
      isMajorChange: false,
    }
    loaded.value = true
  } catch (error) {
    failure.value = messageOf(error)
  }
})
</script>

<template>
  <section class="pb-prompt-form">
    <header class="pb-prompt-form__head">
      <span class="pb-eyebrow" aria-hidden="true">Compose</span>
      <h1 class="pb-prompt-form__title">
        {{ isEdit ? t('prompt.editTitle') : t('prompt.uploadTitle') }}
      </h1>
    </header>

    <el-form
      class="pb-prompt-form__card"
      data-testid="prompt-form"
      label-position="top"
      @submit.prevent="submit"
    >
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
        <el-form-item :label="t('prompt.purpose')">
          <el-select
            v-model="form.purposeID"
            data-testid="prompt-purpose"
            :placeholder="t('prompt.purposePlaceholder')"
          >
            <el-option
              v-for="item in purposeOptions"
              :key="item.id"
              :label="localized(item.name_zh, item.name_en)"
              :value="item.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('prompt.capability')">
          <el-select
            v-model="form.capabilityID"
            data-testid="prompt-capability"
            :placeholder="t('prompt.capabilityPlaceholder')"
          >
            <el-option
              v-for="item in dictionary.data.capabilities"
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
            <el-option-group
              v-for="group in tagGroups"
              :key="group.label"
              :label="group.label"
            >
              <el-option
                v-for="item in group.options"
                :key="item.id"
                :label="localized(item.name_zh, item.name_en)"
                :value="item.id"
              />
            </el-option-group>
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
          :disabled="isEdit && !loaded"
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
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  max-width: 920px;
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-prompt-form__head {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-prompt-form__head::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 140px;
  height: 2px;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
}

.pb-prompt-form__title {
  font-size: 34px;
  font-weight: 500;
  letter-spacing: -0.012em;
  margin: 0;
}

.pb-prompt-form__card {
  padding: var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
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
  color: var(--pb-warning);
  font-size: 13px;
  margin: var(--pb-space-xs) 0 0;
}

.pb-prompt-form__actions {
  display: flex;
  align-items: center;
  gap: var(--pb-space-sm);
  margin-top: var(--pb-space-sm);
  padding-top: var(--pb-space-lg);
  border-top: 1px solid var(--pb-line);
}

.pb-prompt-form__actions :deep(.el-button:last-child) {
  margin-left: auto;
}

.pb-prompt-form__notice {
  margin: var(--pb-space-sm) 0 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--el-color-success-light-9);
  color: var(--pb-success);
  font-size: 13.5px;
}

.pb-prompt-form__failure {
  margin: var(--pb-space-sm) 0 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}
</style>