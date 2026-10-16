<script setup lang="ts">
// 二次确认对话框。破坏性操作统一走这里，使确认动作可被键盘触达，
// 文案由调用方从语言包传入。

const props = defineProps<{
  modelValue: boolean
  title: string
  message: string
  confirmText: string
  cancelText: string
  /** testid 是确认按钮的标识后缀，使多个确认框在同一页面内可区分。 */
  testid: string
}>()

const emit = defineEmits<{
  'update:modelValue': [boolean]
  confirm: []
}>()

function close(): void {
  emit('update:modelValue', false)
}

/**
 * confirm 先抛出确认事件再关闭对话框。
 * 顺序不能颠倒：关闭会把调用方的待确认对象清空，先关再确认会让动作丢失。
 */
function confirm(): void {
  emit('confirm')
  close()
}
</script>

<template>
  <el-dialog
    :model-value="props.modelValue"
    :title="props.title"
    width="420px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <p :data-testid="`${props.testid}-message`">{{ props.message }}</p>
    <template #footer>
      <el-button :data-testid="`${props.testid}-cancel`" @click="close">
        {{ props.cancelText }}
      </el-button>
      <el-button type="danger" :data-testid="`${props.testid}-confirm`" @click="confirm">
        {{ props.confirmText }}
      </el-button>
    </template>
  </el-dialog>
</template>
