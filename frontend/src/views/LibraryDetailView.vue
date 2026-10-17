<script setup lang="ts">
// 题库详情：题目、成员、导出与回收站。
// 角色决定可用的操作（需求 §10.4、§10.5），真正的权限判定在后端。

import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  capabilitiesFor,
  type Library,
  type LibraryMember,
  type LibraryQuestion,
  libraryApi,
  type RecycledQuestion,
} from '@/api/library'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import {
  downloadText,
  kindKey,
  localizedCapability,
  localizedPurpose,
  remainingDays,
  sourceMissing,
} from '@/views/libraryText'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const libraryId = computed(() => Number.parseInt(String(route.params.id), 10))

const library = ref<Library | null>(null)
const questions = ref<LibraryQuestion[]>([])
const members = ref<LibraryMember[]>([])
const recycled = ref<RecycledQuestion[]>([])
const loading = ref(true)
const failure = ref('')
const notice = ref('')

const capabilities = computed(() =>
  capabilitiesFor(library.value?.role === 'owner' ? 'owner' : 'collaborator'),
)

const editDialogVisible = ref(false)
const editForm = ref({ name: '', description: '' })

const addDialogVisible = ref(false)
const addMode = ref<'reference' | 'derived'>('reference')
const addForm = ref({
  promptId: '',
  title: '',
  body: '',
  description: '',
  tags: '',
})

const inviteDialogVisible = ref(false)
const inviteName = ref('')

const purgeLibraryVisible = ref(false)
const purgeQuestionTarget = ref<RecycledQuestion | null>(null)
const deleteQuestionTarget = ref<LibraryQuestion | null>(null)
const removeMemberTarget = ref<LibraryMember | null>(null)

async function load(): Promise<void> {
  loading.value = true
  try {
    const [detail, questionPage, memberList, bin] = await Promise.all([
      libraryApi.detail(libraryId.value),
      libraryApi.questions(libraryId.value, 1, 100),
      libraryApi.members(libraryId.value),
      libraryApi.recycleQuestions(libraryId.value),
    ])
    library.value = detail.library
    questions.value = questionPage.items
    members.value = memberList.items
    recycled.value = bin.items
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

function openEdit(): void {
  if (library.value === null) {
    return
  }
  editForm.value = { name: library.value.name, description: library.value.description }
  editDialogVisible.value = true
}

async function submitEdit(): Promise<void> {
  try {
    // 协作者没有修改名称的能力，因此名称字段不参与提交，避免整次请求被拒。
    const patch = capabilities.value.updateName
      ? { name: editForm.value.name, description: editForm.value.description }
      : { description: editForm.value.description }
    await libraryApi.update(libraryId.value, patch)
    notice.value = t('library.updated')
    editDialogVisible.value = false
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function confirmDeleteLibrary(): Promise<void> {
  purgeLibraryVisible.value = false
  try {
    await libraryApi.remove(libraryId.value)
    await router.push('/libraries')
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function exportLibrary(): Promise<void> {
  try {
    const result = await libraryApi.export(libraryId.value)
    downloadText(result.filename, result.content)
    notice.value = t('library.exported')
  } catch (error) {
    failure.value = messageOf(error)
  }
}

function openAdd(mode: 'reference' | 'derived'): void {
  addMode.value = mode
  addForm.value = { promptId: '', title: '', body: '', description: '', tags: '' }
  addDialogVisible.value = true
}

/** splitTags 把逗号分隔的输入拆成标签数组。 */
function splitTags(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((item) => item.trim())
    .filter((item) => item !== '')
}

async function submitAdd(): Promise<void> {
  try {
    if (addMode.value === 'reference') {
      const promptId = Number.parseInt(addForm.value.promptId, 10)
      if (Number.isNaN(promptId)) {
        failure.value = t('library.promptIdHint')
        return
      }
      await libraryApi.addQuestion(libraryId.value, {
        prompt_id: promptId,
        tags: splitTags(addForm.value.tags),
      })
      notice.value = t('library.referenceAdded')
    } else {
      await libraryApi.addQuestion(libraryId.value, {
        title: addForm.value.title,
        body: addForm.value.body,
        description: addForm.value.description,
        tags: splitTags(addForm.value.tags),
      })
      notice.value = t('library.derivedAdded')
    }
    addDialogVisible.value = false
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function move(index: number, offset: number): Promise<void> {
  const target = index + offset
  if (target < 0 || target >= questions.value.length) {
    return
  }
  const reordered = [...questions.value]
  const current = reordered[index] as LibraryQuestion
  reordered[index] = reordered[target] as LibraryQuestion
  reordered[target] = current
  try {
    await libraryApi.reorder(
      libraryId.value,
      reordered.map((item) => item.id),
    )
    notice.value = t('library.reordered')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function confirmDeleteQuestion(): Promise<void> {
  const target = deleteQuestionTarget.value
  deleteQuestionTarget.value = null
  if (target === null) {
    return
  }
  try {
    await libraryApi.removeQuestion(target.id)
    notice.value = t('library.questionDeleted')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function confirmRemoveMember(): Promise<void> {
  const target = removeMemberTarget.value
  removeMemberTarget.value = null
  if (target === null) {
    return
  }
  try {
    await libraryApi.removeMember(libraryId.value, target.user_id)
    notice.value = t('library.memberRemoved')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function submitInvite(): Promise<void> {
  try {
    await libraryApi.invite(libraryId.value, inviteName.value)
    notice.value = t('library.invited', { name: inviteName.value })
    inviteDialogVisible.value = false
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function restoreQuestion(entry: RecycledQuestion): Promise<void> {
  try {
    await libraryApi.restoreQuestion(entry.question.id)
    notice.value = t('library.questionRestored')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function confirmPurgeQuestion(): Promise<void> {
  const target = purgeQuestionTarget.value
  purgeQuestionTarget.value = null
  if (target === null) {
    return
  }
  try {
    await libraryApi.purgeQuestion(target.question.id)
    notice.value = t('library.questionPurged')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** remainingLabel 把到期时刻转为可读的剩余天数。 */
function remainingLabel(expiresAt: string): string {
  const days = remainingDays(expiresAt)
  return days === 0 ? t('library.expiresToday') : t('library.remainingDays', { days })
}

onMounted(async () => {
  await auth.connect()
  await load()
})

// 同路由参数切换时组件复用：复位全部状态并重载，防止展示与操作错位。
watch(
  () => route.params.id,
  () => {
    library.value = null
    questions.value = []
    members.value = []
    recycled.value = []
    loading.value = true
    failure.value = ''
    notice.value = ''
    void load()
  },
)
</script>

<template>
  <section class="pb-library">
    <p v-if="failure" class="pb-library__error" data-testid="library-error">{{ failure }}</p>
    <p v-if="notice" class="pb-library__notice" data-testid="library-notice">{{ notice }}</p>
    <p v-if="loading" class="pb-library__hint">{{ t('library.title') }}</p>

    <template v-else-if="library">
      <header class="pb-library__head">
        <div class="pb-library__heading">
          <span class="pb-eyebrow" aria-hidden="true">Library</span>
          <h1 class="pb-library__title" data-testid="library-name">{{ library.name }}</h1>
          <p class="pb-library__meta" data-testid="library-meta">
            {{ t(library.role === 'owner' ? 'library.roleOwner' : 'library.roleCollaborator') }} ·
            {{ t('library.capacity', { used: library.question_count, limit: library.question_limit }) }} ·
            {{ t('library.liveQuestions', { count: library.live_question_count }) }}
            <template v-if="library.owner_name">
              · {{ t('library.owner', { name: library.owner_name }) }}
            </template>
          </p>
        </div>
        <div class="pb-library__actions">
          <el-button
            v-if="capabilities.updateDescription || capabilities.updateName"
            size="small"
            data-testid="library-edit"
            @click="openEdit"
          >
            {{ t('library.save') }}
          </el-button>
          <el-button size="small" data-testid="library-export" @click="exportLibrary">
            {{ t('library.export') }}
          </el-button>
          <el-button
            v-if="capabilities.deleteLibrary"
            size="small"
            type="danger"
            data-testid="library-delete"
            @click="purgeLibraryVisible = true"
          >
            {{ t('library.deleteLibrary') }}
          </el-button>
        </div>
      </header>

      <p class="pb-library__description" data-testid="library-description">
        {{ library.description }}
      </p>

      <section class="pb-library__section">
        <header class="pb-library__section-head">
          <h2 class="pb-library__subtitle">{{ t('library.questions') }}</h2>
          <div v-if="capabilities.addQuestion" class="pb-library__section-actions">
            <el-button size="small" data-testid="library-add-reference" @click="openAdd('reference')">
              {{ t('library.addReference') }}
            </el-button>
            <el-button size="small" data-testid="library-add-derived" @click="openAdd('derived')">
              {{ t('library.addDerived') }}
            </el-button>
          </div>
        </header>

        <p v-if="questions.length === 0" data-testid="library-questions-empty">
          {{ t('library.questionsEmpty') }}
        </p>
        <ol v-else class="pb-library__questions" data-testid="library-questions">
          <li
            v-for="(question, index) in questions"
            :key="question.id"
            class="pb-library__question"
            :data-testid="`question-${question.id}`"
          >
            <RouterLink class="pb-library__question-title" :to="`/library-questions/${question.id}`">
              {{ question.title }}
            </RouterLink>
            <span class="pb-library__question-meta" :data-testid="`question-meta-${question.id}`">
              {{ t(kindKey(question.kind)) }} ·
              {{ localizedPurpose(question, locale) }}
              <template v-if="question.capability_name_zh">
                · {{ localizedCapability(question, locale) }}
              </template>
              <template v-if="question.tags.length > 0"> · {{ question.tags.join('、') }}</template>
            </span>
            <p
              v-if="sourceMissing(question)"
              class="pb-library__warning"
              :data-testid="`question-source-missing-${question.id}`"
            >
              {{ t('library.sourceMissing') }}
            </p>
            <div class="pb-library__question-actions">
              <template v-if="capabilities.reorderQuestion">
                <el-button
                  size="small"
                  text
                  :disabled="index === 0"
                  :data-testid="`question-up-${question.id}`"
                  @click="move(index, -1)"
                >
                  {{ t('library.moveUp') }}
                </el-button>
                <el-button
                  size="small"
                  text
                  :disabled="index === questions.length - 1"
                  :data-testid="`question-down-${question.id}`"
                  @click="move(index, 1)"
                >
                  {{ t('library.moveDown') }}
                </el-button>
              </template>
              <el-button
                v-if="capabilities.deleteQuestion"
                size="small"
                text
                type="danger"
                :data-testid="`question-delete-${question.id}`"
                @click="deleteQuestionTarget = question"
              >
                {{ t('library.deleteQuestion') }}
              </el-button>
            </div>
          </li>
        </ol>
      </section>

      <section class="pb-library__section">
        <header class="pb-library__section-head">
          <h2 class="pb-library__subtitle">
            {{ t('library.members') }}
            <span class="pb-library__count">{{ t('library.memberCount', { count: members.length }) }}</span>
          </h2>
          <el-button
            v-if="capabilities.inviteMember"
            size="small"
            data-testid="library-invite"
            @click="inviteDialogVisible = true"
          >
            {{ t('library.invite') }}
          </el-button>
        </header>
        <ul class="pb-library__members" data-testid="library-members">
          <li
            v-for="member in members"
            :key="member.user_id"
            class="pb-library__member"
            :data-testid="`member-${member.user_id}`"
          >
            <span>{{ member.username }}</span>
            <span class="pb-library__question-meta">
              {{ t(member.role === 'owner' ? 'library.roleOwner' : 'library.roleCollaborator') }}
            </span>
            <el-button
              v-if="capabilities.removeMember && member.role !== 'owner'"
              size="small"
              text
              type="danger"
              :data-testid="`member-remove-${member.user_id}`"
              @click="removeMemberTarget = member"
            >
              {{ t('library.removeMember') }}
            </el-button>
          </li>
        </ul>
      </section>

      <section class="pb-library__section">
        <h2 class="pb-library__subtitle">{{ t('library.questionRecycleBin') }}</h2>
        <p v-if="recycled.length === 0" data-testid="library-question-recycle-empty">
          {{ t('library.questionRecycleBinEmpty') }}
        </p>
        <ul v-else class="pb-library__recycle" data-testid="library-question-recycle">
          <li
            v-for="entry in recycled"
            :key="entry.question.id"
            class="pb-library__member"
            :data-testid="`recycled-question-${entry.question.id}`"
          >
            <span>{{ entry.question.title }}</span>
            <span class="pb-library__question-meta" :data-testid="`recycled-meta-${entry.question.id}`">
              {{ remainingLabel(entry.expires_at) }} · {{ t('library.deletedBy', { id: entry.deleted_by }) }}
            </span>
            <el-button
              v-if="capabilities.restoreQuestion"
              size="small"
              :data-testid="`question-restore-${entry.question.id}`"
              @click="restoreQuestion(entry)"
            >
              {{ t('library.restore') }}
            </el-button>
            <el-button
              v-if="capabilities.purgeQuestion"
              size="small"
              type="danger"
              :data-testid="`question-purge-${entry.question.id}`"
              @click="purgeQuestionTarget = entry"
            >
              {{ t('library.purge') }}
            </el-button>
          </li>
        </ul>
      </section>
    </template>

    <el-dialog v-model="editDialogVisible" :title="t('library.save')" width="480px">
      <el-form label-width="90px">
        <el-form-item v-if="capabilities.updateName" :label="t('library.name')">
          <el-input v-model="editForm.name" maxlength="100" data-testid="library-edit-name" />
        </el-form-item>
        <el-form-item :label="t('library.description')">
          <el-input
            v-model="editForm.description"
            type="textarea"
            :rows="3"
            maxlength="2000"
            data-testid="library-edit-description"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button data-testid="library-edit-cancel" @click="editDialogVisible = false">
          {{ t('auth.cancel') }}
        </el-button>
        <el-button type="primary" data-testid="library-edit-submit" @click="submitEdit">
          {{ t('library.save') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="addDialogVisible"
      :title="addMode === 'reference' ? t('library.addReference') : t('library.addDerived')"
      width="560px"
    >
      <p class="pb-library__hint">
        {{ addMode === 'reference' ? t('library.promptIdHint') : t('library.editHint') }}
      </p>
      <el-form label-width="110px">
        <el-form-item v-if="addMode === 'reference'" :label="t('library.promptId')">
          <el-input v-model="addForm.promptId" data-testid="library-add-prompt-id" />
        </el-form-item>
        <template v-else>
          <el-form-item :label="t('library.questionTitle')">
            <el-input v-model="addForm.title" maxlength="200" data-testid="library-add-title" />
          </el-form-item>
          <el-form-item :label="t('library.questionBody')">
            <el-input
              v-model="addForm.body"
              type="textarea"
              :rows="6"
              data-testid="library-add-body"
            />
          </el-form-item>
          <el-form-item :label="t('library.questionDescription')">
            <el-input
              v-model="addForm.description"
              type="textarea"
              :rows="2"
              maxlength="2000"
              data-testid="library-add-description"
            />
          </el-form-item>
        </template>
        <el-form-item :label="t('library.questionTags')">
          <el-input v-model="addForm.tags" data-testid="library-add-tags" />
          <span class="pb-library__hint">{{ t('library.tagsHint') }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button data-testid="library-add-cancel" @click="addDialogVisible = false">
          {{ t('auth.cancel') }}
        </el-button>
        <el-button type="primary" data-testid="library-add-submit" @click="submitAdd">
          {{ t('library.addQuestion') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="inviteDialogVisible" :title="t('library.invite')" width="420px">
      <p class="pb-library__hint">{{ t('library.inviteHint') }}</p>
      <el-input v-model="inviteName" :placeholder="t('library.inviteHint')" data-testid="library-invite-name" />
      <template #footer>
        <el-button data-testid="library-invite-cancel" @click="inviteDialogVisible = false">
          {{ t('auth.cancel') }}
        </el-button>
        <el-button type="primary" data-testid="library-invite-submit" @click="submitInvite">
          {{ t('library.inviteSubmit') }}
        </el-button>
      </template>
    </el-dialog>

    <ConfirmDialog
      :model-value="purgeLibraryVisible"
      :title="t('library.deleteLibrary')"
      :message="t('library.deleteConfirm')"
      :confirm-text="t('library.deleteLibrary')"
      :cancel-text="t('auth.cancel')"
      testid="library-delete"
      @update:model-value="purgeLibraryVisible = $event"
      @confirm="confirmDeleteLibrary"
    />

    <ConfirmDialog
      :model-value="deleteQuestionTarget !== null"
      :title="t('library.deleteQuestion')"
      :message="t('library.deleteQuestionConfirm')"
      :confirm-text="t('library.deleteQuestion')"
      :cancel-text="t('auth.cancel')"
      testid="question-delete"
      @update:model-value="
        (value: boolean) => {
          if (!value) deleteQuestionTarget = null
        }
      "
      @confirm="confirmDeleteQuestion"
    />

    <ConfirmDialog
      :model-value="purgeQuestionTarget !== null"
      :title="t('library.purge')"
      :message="t('library.purgeConfirm')"
      :confirm-text="t('library.purge')"
      :cancel-text="t('auth.cancel')"
      testid="question-purge"
      @update:model-value="
        (value: boolean) => {
          if (!value) purgeQuestionTarget = null
        }
      "
      @confirm="confirmPurgeQuestion"
    />

    <ConfirmDialog
      :model-value="removeMemberTarget !== null"
      :title="t('library.removeMember')"
      :message="t('library.removeMemberConfirm')"
      :confirm-text="t('library.removeMember')"
      :cancel-text="t('auth.cancel')"
      testid="member-remove"
      @update:model-value="
        (value: boolean) => {
          if (!value) removeMemberTarget = null
        }
      "
      @confirm="confirmRemoveMember"
    />
  </section>
</template>

<style scoped>
.pb-library {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-library__head {
  position: relative;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-library__head::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 140px;
  height: 2px;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
}

.pb-library__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  min-width: 0;
}

.pb-library__title {
  font-size: 34px;
  font-weight: 500;
  letter-spacing: -0.012em;
  margin: 0;
}

.pb-library__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  justify-content: flex-end;
}

.pb-library__subtitle {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.pb-library__count {
  margin-left: var(--pb-space-xs);
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-library__description {
  max-width: 72ch;
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 15px;
  line-height: 1.7;
}

.pb-library__section {
  margin: 0;
  padding: var(--pb-space-lg) var(--pb-space-xl) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
}

.pb-library__section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-md);
  flex-wrap: wrap;
}

.pb-library__section-actions {
  display: flex;
  gap: var(--pb-space-xs);
}

.pb-library__meta,
.pb-library__hint,
.pb-library__question-meta {
  display: block;
  margin: 0;
  color: var(--pb-ink-faint);
  font-size: 12.5px;
}

.pb-library__questions,
.pb-library__members,
.pb-library__recycle {
  list-style: none;
  margin: var(--pb-space-md) 0 0;
  padding: 0;
}

.pb-library__question,
.pb-library__member {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
  padding: var(--pb-space-sm) 0;
  border-bottom: 1px solid var(--pb-line);
}

.pb-library__question:last-child,
.pb-library__member:last-child {
  border-bottom: 0;
}

.pb-library__question-title {
  flex: 1;
  min-width: 0;
  color: var(--pb-ink);
  font-size: 15px;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.pb-library__question-title:hover {
  color: var(--pb-accent);
}

.pb-library__question-actions {
  display: flex;
  gap: var(--pb-space-xs);
}

.pb-library__warning {
  flex-basis: 100%;
  margin: 0;
  color: var(--pb-warning);
  font-size: 12px;
}

.pb-library__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

.pb-library__notice,
.pb-library__hint {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
}

@media (max-width: 720px) {
  .pb-library__head {
    flex-direction: column;
    align-items: flex-start;
  }

  .pb-library__actions {
    justify-content: flex-start;
  }
}
</style>
