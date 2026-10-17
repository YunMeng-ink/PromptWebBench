<script setup lang="ts">
// 题库题目详情：正文、标签、修改记录，以及复制按钮（需求 §10.8）。

import { computed, onMounted, ref, watch } from 'vue'
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
  localizedCapability,
  localizedPurpose,
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
  purpose: t('library.fieldPurpose'),
  capability: t('library.fieldCapability'),
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
        purpose: localizedPurpose(current, locale.value),
        capability: localizedCapability(current, locale.value),
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

// 同路由参数切换时组件复用：复位全部状态并重载，防止展示与操作错位。
watch(questionId, () => {
  question.value = null
  revisions.value = []
  loading.value = true
  failure.value = ''
  notice.value = ''
  void load()
})
</script>

<template>
  <section class="pb-question">
    <p v-if="failure" class="pb-question__error" data-testid="question-error">{{ failure }}</p>
    <p v-if="notice" class="pb-question__notice" data-testid="question-notice">{{ notice }}</p>
    <p v-if="loading" class="pb-question__hint">{{ t('library.questions') }}</p>

    <template v-else-if="question">
      <header class="pb-question__head">
        <div class="pb-question__heading">
          <span class="pb-eyebrow" aria-hidden="true">Question</span>
          <h1 class="pb-question__title" data-testid="question-title">{{ question.title }}</h1>
        </div>
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
        <dt>{{ t('library.fieldPurpose') }}</dt>
        <dd data-testid="question-purpose">{{ localizedPurpose(question, locale) }}</dd>
        <dt>{{ t('library.fieldCapability') }}</dt>
        <dd data-testid="question-capability">{{ localizedCapability(question, locale) }}</dd>
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
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-question__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-question__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  min-width: 0;
}

.pb-question__title {
  font-size: 30px;
  font-weight: 600;
  letter-spacing: 0.01em;
  margin: 0;
}

.pb-question__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  justify-content: flex-end;
}

.pb-question__subtitle {
  margin: 0 0 var(--pb-space-sm);
  font-size: 18px;
  font-weight: 600;
}

.pb-question__section {
  margin: 0;
  padding: var(--pb-space-lg) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
}

.pb-question__facts {
  display: grid;
  gap: var(--pb-space-md) var(--pb-space-xl);
  grid-template-columns: max-content 1fr;
  margin: 0;
  padding: var(--pb-space-lg) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
}

.pb-question__facts dt {
  color: var(--pb-ink-faint);
  font-size: 11.5px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.pb-question__facts dd {
  margin: 0;
  color: var(--pb-ink);
  font-size: 14px;
}

.pb-question__tags,
.pb-question__revisions {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-question__tags li {
  display: flex;
  align-items: center;
  gap: var(--pb-space-xs);
  padding: 4px 0;
  border-bottom: 1px solid var(--pb-line);
}

.pb-question__tags li:last-child {
  border-bottom: 0;
}

.pb-question__tag-form {
  display: flex;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-sm);
  max-width: 420px;
}

.pb-question__body {
  margin: 0;
  padding: var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-left: 3px solid var(--pb-accent);
  border-radius: var(--pb-radius-lg);
  font-family: var(--pb-font-mono);
  font-size: 13.5px;
  line-height: 1.85;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.pb-question__warning {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  background: var(--el-color-warning-light-9);
  color: var(--pb-warning);
  font-size: 13.5px;
}

.pb-question__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

.pb-question__notice,
.pb-question__hint {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
}

@media (max-width: 720px) {
  .pb-question__head {
    flex-direction: column;
    align-items: flex-start;
  }

  .pb-question__actions {
    justify-content: flex-start;
  }
}
</style>
