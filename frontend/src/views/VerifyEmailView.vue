<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { authApi } from '@/api/auth'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const email = computed(() => (typeof route.query.email === 'string' ? route.query.email : ''))
const code = ref('')
const submitting = ref(false)
const resending = ref(false)
const errorText = ref('')
const noticeText = ref(t('auth.registerSuccess'))

async function submit(): Promise<void> {
  submitting.value = true
  errorText.value = ''
  try {
    await auth.verifyEmail(email.value, code.value)
    await router.push('/')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}

async function resend(): Promise<void> {
  resending.value = true
  errorText.value = ''
  try {
    await authApi.resendVerification({ email: email.value })
    noticeText.value = t('auth.codeSent')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    resending.value = false
  }
}
</script>

<template>
  <section class="pb-auth">
    <h1 class="pb-auth__title">{{ t('auth.verifyTitle') }}</h1>
    <p class="pb-auth__hint">{{ t('auth.verifyHint', { email }) }}</p>

    <el-alert v-if="errorText" class="pb-auth__error" type="error" :title="errorText" show-icon />
    <el-alert v-else class="pb-auth__notice" type="info" :title="noticeText" show-icon />

    <el-form label-position="top" @submit.prevent="submit">
      <el-form-item :label="t('auth.code')">
        <el-input
          v-model="code"
          maxlength="6"
          autocomplete="one-time-code"
          data-testid="verify-code"
        />
      </el-form-item>
      <el-button
        type="primary"
        native-type="submit"
        :loading="submitting"
        data-testid="verify-submit"
      >
        {{ t('auth.submitVerify') }}
      </el-button>
      <el-button :loading="resending" data-testid="verify-resend" @click="resend">
        {{ t('auth.resend') }}
      </el-button>
    </el-form>
  </section>
</template>

<style scoped>
.pb-auth {
  max-width: 420px;
  margin: 0 auto;
  padding: var(--pb-space-xl) 0;
}

.pb-auth__title {
  font-size: 24px;
  margin: 0 0 var(--pb-space-sm);
}

.pb-auth__hint {
  color: var(--pb-color-text-soft);
  margin: 0 0 var(--pb-space-lg);
}

.pb-auth__error,
.pb-auth__notice {
  margin-bottom: var(--pb-space-md);
}
</style>
