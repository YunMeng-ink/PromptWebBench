// 举报对话框。举报原因以英文枚举键提交（C-31），界面文案按当前语言取语言包，
// 因此英文界面下显示英文原因文案而不需要后端改动。

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { REPORT_REASONS, type ReportObjectType, type ReportReason, socialApi } from '@/api/social'
import { useErrorMessage } from '@/composables/useErrorMessage'

const props = defineProps<{
  visible: boolean
  objectType: ReportObjectType
  objectId: number
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  submitted: []
}>()

const { t } = useI18n()
const { messageOf } = useErrorMessage()

const reason = ref<ReportReason>('bad_content')
const detail = ref('')
const failure = ref('')
const notice = ref('')
const submitting = ref(false)

/** reasonLabel 把英文枚举键映射到当前语言的文案。 */
function reasonLabel(value: ReportReason): string {
  switch (value) {
    case 'bad_content':
      return t('prompt.reportReasonBadContent')
    case 'illegal':
      return t('prompt.reportReasonIllegal')
    case 'porn':
      return t('prompt.reportReasonPorn')
    case 'attack':
      return t('prompt.reportReasonAttack')
    case 'privacy':
      return t('prompt.reportReasonPrivacy')
    case 'spam':
      return t('prompt.reportReasonSpam')
    case 'dangerous':
      return t('prompt.reportReasonDangerous')
    default:
      return t('prompt.reportReasonOther')
  }
}

/** 每次打开都重置，避免上一次的输入与提示残留。 */
watch(
  () => props.visible,
  (value) => {
    if (value) {
      reason.value = 'bad_content'
      detail.value = ''
      failure.value = ''
      notice.value = ''
    }
  },
)

async function submit(): Promise<void> {
  submitting.value = true
  try {
    await socialApi.report({
      object_type: props.objectType,
      object_id: props.objectId,
      reason: reason.value,
      detail: detail.value,
    })
    notice.value = t('prompt.reportSubmitted')
    emit('submitted')
    emit('update:visible', false)
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}

function close(): void {
  emit('update:visible', false)
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('prompt.reportTitle')"
    width="440px"
    @update:model-value="close"
  >
    <el-form label-width="90px">
      <el-form-item :label="t('prompt.reportReason')">
        <el-radio-group v-model="reason" data-testid="report-reason">
          <el-radio v-for="value in REPORT_REASONS" :key="value" :value="value" :data-testid="`report-reason-${value}`">
            {{ reasonLabel(value) }}
          </el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item :label="t('prompt.reportDetail')">
        <el-input
          v-model="detail"
          type="textarea"
          :rows="3"
          maxlength="500"
          data-testid="report-detail"
        />
      </el-form-item>
    </el-form>

    <p v-if="failure" class="pb-report__error" data-testid="report-error">{{ failure }}</p>
    <p v-if="notice" class="pb-report__notice" data-testid="report-notice">{{ notice }}</p>

    <template #footer>
      <el-button data-testid="report-cancel" @click="close">{{ t('auth.cancel') }}</el-button>
      <el-button
        type="primary"
        :loading="submitting"
        data-testid="report-submit"
        @click="submit"
      >
        {{ t('prompt.reportSubmit') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.pb-report__error {
  color: var(--el-color-danger);
}

.pb-report__notice {
  color: var(--pb-color-text-soft);
}
</style>