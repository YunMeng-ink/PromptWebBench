<script setup lang="ts">
// 题库题目详情：正文、标签、修改记录，以及复制按钮（需求 §10.8）。

import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import {
  capabilitiesFor,
  type LibraryQuestion,
  libraryApi,
  type QuestionRevision,
} from '@/api/library'
import { useClipboard } from '@/composables/useClipboard'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import {
  type CopyLabels,
  kindKey,
  localizedCategory,
  PLACEHOLDER,
  promptBodyText,
  questionFullText,
  sourceLabel,
  sourceMissing,
} from '@/views/libraryText'

const route = useRoute()
const { t, locale } = useI18n()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()
const { copied, copy } = useClipboard()

const questionId = computed(() => Number.parseInt(String(route.params.id), 10))

const question = ref<LibraryQuestion | null>(null)
const revisions = ref<QuestionRevision[]>([])
const role = ref<'owner' | 'collaborator'>('owner')
const loading = ref(true)
const failure = ref('')
const notice = ref('')
const copyFailure = ref('')

const editDialogVisible = ref(false)
const editForm = ref({ title: '', body: '', description: '', note: '' })
const tagInput = ref('')

const capabilities = computed(() => capabilitiesFor(role.value))

/** copyLabels 是复制文本里的字段名，全部取自语言包。 */
const copyLabels = computed<CopyLabels>(() => ({
  title: t('library.fieldTitle'),
  category: t('library.fieldCategory'),
  version: t('library.fieldVersion'),
  tags: t('library.fieldTags'),
  description: t('library.fieldDescription'),
  kind: t('library.fieldKind'),
  source: t('library.fieldSource'),
  body: t('library.fieldBody'),
}))

async function load(): Promise<void> {
  loading.value = true
  try {
    const detail = await libraryApi.question(questionId.value)
    question.value = detail.question
    const [library, history] = await Promise.all([
      libraryApi.detail(detail.question.library_id),
      libraryApi.revisions(questionId.value),
    ])
    role.value = library.library.role
    revisions.value = history.items
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

/** copyBody 复制提示词正文。 */
async function copyBody(): Promise<void> {
  if (question.value === null) {
    return
  }
  await runCopy(promptBodyText(question.value.body))
}

/** copyFull 复制带字段名的完整信息。 */
async function copyFull(): Promise<void> {
  if (question.value === null) {
    return
  }
  const current = question.value
  await runCopy(
    questionFullText(
      {
        title: current.title,
        kindName: t(kindKey(current.kind)),
        category: localizedCategory(current, locale.value),
        tags: current.tags,
        description: current.description,
        source: sourceLabel(current),
        body: current.body,
      },
      copyLabels.value,
    ),
  )
}

async function runCopy(text: string): Promise<void> {
  const ok = await copy(text, t('home.copied'))
  copyFailure.value = ok ? '' : t('home.copyFailed')
}

function openEdit(): void {
  if (question.value === null) {
    return
  }
  editForm.value = {
    title: question.value.title,
    body: question.value.body,
    description: question.value.description,
    note: '',
  }
  editDialogVisible.value = true
}

async function submitEdit(): Promise<void> {
  try {
    await libraryApi.updateQuestion(questionId.value, {
      title: editForm.value.title,
      body: editForm.value.body,
      description: editForm.value.description,
      change_note: editForm.value.note,
    })
    notice.value = t('library.questionUpdated')
    editDialogVisible.value = false
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function addTag(): Promise<void> {
  const tag = tagInput.value.trim()
  if (tag === '') {
    return
  }
  try {
    await libraryApi.addTag(questionId.value, tag)
    tagInput.value = ''
    notice.value = t('library.tagAdded')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function removeTag(tag: string): Promise<void> {
  try {
    await libraryApi.removeTag(questionId.value, tag)
    notice.value = t('library.tagRemoved')
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
  <section class="pb-question">
    <p v-if="failure" class="pb-question__error" data-testid="question-error">{{ failure }}</p>
    <p v-if="notice" class="pb-question__notice" data-testid="question-notice">{{ notice }}</p>
    <p v-if="loading" class="pb-question__hint">{{ t('library.questions') }}</p>

    <template v-else-if="question">
      <header class="pb-question__head">
        <h1 class="pb-question__title" data-testid="question-title">{{ question.title }}</h1>
        <div class="pb-question__actions">
          <el-button size="small" data-testid="question-copy-body" @click="copyBody">
            {{ t('home.copyBody') }}
          </el-button>
          <el-button size="small" data-testid="question-copy-full" @click="copyFull">
            {{ t('home.copyFull') }}
          </el-button>
          <el-button
            v-if="capabilities.updateQuestion"
            size="small"
            data-testid="question-edit"
            @click="openEdit"
          >
            {{ t('library.editQuestion') }}
          </el-button>
        </div>
      </header>

      <p v-if="copied" class="pb-question__notice" data-testid="question-copied">{{ copied }}</p>
      <p v-if="copyFailure" class="pb-question__error" data-testid="question-copy-failed">
        {{ copyFailure }}
      </p>

      <dl class="pb-question__facts">
        <dt>{{ t('library.fieldKind') }}</dt>
        <dd data-testid="question-kind">{{ t(kindKey(question.kind)) }}</dd>
        <dt>{{ t('library.fieldCategory') }}</dt>
        <dd data-testid="question-category">{{ localizedCategory(question, locale) }}</dd>
        <dt>{{ t('library.fieldSource') }}</dt>
        <dd data-testid="question-source">{{ sourceLabel(question) }}</dd>
      </dl>

      <p
        v-if="sourceMissing(question)"
        class="pb-question__warning"
        data-testid="question-source-missing"
      >
        {{ t('library.sourceMissing') }}
      </p>
      <p v-else-if="question.source_major_modified" class="pb-question__warning">
        {{ t('library.sourceMajorModified') }}
      </p>

      <section class="pb-question__section">
        <h2 class="pb-question__subtitle">{{ t('library.tags') }}</h2>
        <ul class="pb-question__tags" data-testid="question-tags">
          <li v-for="tag in question.tags" :key="tag" :data-testid="`tag-${tag}`">
            <span>{{ tag }}</span>
            <el-button
              v-if="capabilities.tagQuestion"
              size="small"
              text
              type="danger"
              :data-testid="`tag-remove-${tag}`"
              @click="removeTag(tag)"
            >
              {{ t('library.removeTag') }}
            </el-button>
          </li>
        </ul>
        <p v-if="question.tags.length === 0" class="pb-question__hint" data-testid="question-tags-empty">
          {{ PLACEHOLDER }}
        </p>
        <div v-if="capabilities.tagQuestion" class="pb-question__tag-form">
          <el-input v-model="tagInput" maxlength="50" data-testid="question-tag-input" />
          <el-button size="small" data-testid="question-tag-add" @click="addTag">
            {{ t('library.addTag') }}
          </el-button>
        </div>
      </section>

      <section class="pb-question__section">
        <h2 class="pb-question__subtitle">{{ t('library.fieldBody') }}</h2>
        <pre class="pb-question__body" data-testid="question-body">{{ question.body }}</pre>
      </section>

      <section v-if="question.description" class="pb-question__section">
        <h2 class="pb-question__subtitle">{{ t('library.questionDescription') }}</h2>
        <p data-testid="question-description">{{ question.description }}</p>
      </section>

      <section class="pb-question__section">
        <h2 class="pb-question__subtitle">{{ t('library.revisions') }}</h2>
        <p v-if="revisions.length === 0" class="pb-question__hint" data-testid="question-revisions-empty">
          {{ t('library.revisionsEmpty') }}
        </p>
        <ul v-else class="pb-question__revisions" data-testid="question-revisions">
          <li
            v-for="revision in revisions"
            :key="revision.id"
            :data-testid="`revision-${revision.id}`"
          >
            {{ t('library.revisionBy', { name: revision.editor_name, time: revision.created_at }) }}
            <span v-if="revision.note">· {{ revision.note }}</span>
          </li>
        </ul>
      </section>
    </template>

    <el-dialog v-model="editDialogVisible" :title="t('library.editQuestion')" width="600px">
      <p class="pb-question__hint">{{ t('library.editHint') }}</p>
      <el-form label-width="90px">
        <el-form-item :label="t('library.questionTitle')">
          <el-input v-model="editForm.title" maxlength="200" data-testid="question-edit-title" />
        </el-form-item>
        <el-form-item :label="t('library.questionBody')">
          <el-input v-model="editForm.body" type="textarea" :rows="8" data-testid="question-edit-body" />
        </el-form-item>
        <el-form-item :label="t('library.questionDescription')">
          <el-input
            v-model="editForm.description"
            type="textarea"
            :rows="2"
            maxlength="2000"
            data-testid="question-edit-description"
          />
        </el-form-item>
        <el-form-item :label="t('library.changeNote')">
          <el-input v-model="editForm.note" maxlength="500" data-testid="question-edit-note" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button data-testid="question-edit-cancel" @click="editDialogVisible = false">
          {{ t('auth.cancel') }}
        </el-button>
        <el-button type="primary" data-testid="question-edit-submit" @click="submitEdit">
          {{ t('library.save') }}
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.pb-question {
  padding: var(--pb-space-lg) 0;
}

.pb-question__head {
  align-items: flex-start;
  display: flex;
  justify-content: space-between;
}

.pb-question__title {
  font-size: 22px;
  margin: 0;
}

.pb-question__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-question__subtitle {
  font-size: 16px;
  margin: 0 0 var(--pb-space-xs);
}

.pb-question__section {
  margin-top: var(--pb-space-lg);
}

.pb-question__facts {
  display: grid;
  gap: var(--pb-space-xs) var(--pb-space-sm);
  grid-template-columns: max-content 1fr;
  margin: var(--pb-space-md) 0 0;
}

.pb-question__facts dt {
  color: var(--pb-color-text-faint);
  font-size: 12px;
}

.pb-question__facts dd {
  margin: 0;
}

.pb-question__tags,
.pb-question__revisions {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-question__tags li {
  align-items: center;
  display: flex;
  gap: var(--pb-space-xs);
}

.pb-question__tag-form {
  display: flex;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-xs);
  max-width: 420px;
}

.pb-question__body {
  background: var(--pb-color-surface, #f7f8fa);
  border-radius: 6px;
  font-family: var(--pb-font-mono, monospace);
  margin: 0;
  overflow-x: auto;
  padding: var(--pb-space-sm);
  white-space: pre-wrap;
  word-break: break-word;
}

.pb-question__warning {
  color: var(--el-color-warning);
}

.pb-question__error {
  color: var(--el-color-danger);
}

.pb-question__notice,
.pb-question__hint {
  color: var(--pb-color-text-soft);
}

@media (max-width: 720px) {
  .pb-question__head {
    flex-direction: column;
    gap: var(--pb-space-sm);
  }
}
</style>
