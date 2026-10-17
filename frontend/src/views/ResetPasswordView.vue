<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { authApi } from '@/api/auth'
import { useErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()
const router = useRouter()
const { messageOf } = useErrorMessage()

const email = ref('')
const code = ref('')
const newPassword = ref('')
const sending = ref(false)
const submitting = ref(false)
const errorText = ref('')
const noticeText = ref('')

async function sendCode(): Promise<void> {
  sending.value = true
  errorText.value = ''
  try {
    await authApi.requestPasswordReset({ email: email.value })
    noticeText.value = t('auth.codeSent')
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    sending.value = false
  }
}

async function submit(): Promise<void> {
  submitting.value = true
  errorText.value = ''
  try {
    await authApi.resetPassword({
      email: email.value,
      code: code.value,
      new_password: newPassword.value,
    })
    await router.push({ name: 'login' })
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="pb-auth">
    <div class="pb-auth__card pb-auth__card--standard">
      <span class="pb-auth__eyebrow" aria-hidden="true">Password Reset</span>
      <h1 class="pb-auth__title">{{ t('auth.resetTitle') }}</h1>
      <p class="pb-auth__hint">{{ t('auth.resetHint') }}</p>

      <el-alert v-if="errorText" class="pb-auth__error" type="error" :title="errorText" show-icon />
      <el-alert
        v-else-if="noticeText"
        class="pb-auth__notice"
        type="info"
        :title="noticeText"
        show-icon
      />

      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item :label="t('auth.email')">
          <el-input v-model="email" type="email" autocomplete="email" data-testid="reset-email" />
        </el-form-item>
        <el-form-item :label="t('auth.code')">
          <el-input v-model="code" maxlength="6" data-testid="reset-code" />
        </el-form-item>
        <el-form-item :label="t('auth.newPassword')">
          <el-input
            v-model="newPassword"
            type="password"
            autocomplete="new-password"
            show-password
            data-testid="reset-password"
          />
        </el-form-item>
        <div class="pb-auth__actions">
          <el-button
            class="pb-auth__action"
            :loading="sending"
            data-testid="reset-send-code"
            @click="sendCode"
          >
            {{ t('auth.sendCode') }}
          </el-button>
          <el-button
            class="pb-auth__action"
            type="primary"
            native-type="submit"
            :loading="submitting"
            data-testid="reset-submit"
          >
            {{ t('auth.submitReset') }}
          </el-button>
        </div>
      </el-form>

      <div class="pb-auth__aside">
        <span class="pb-auth__aside-title">{{ t('auth.resetAsideTitle') }}</span>
        <ul class="pb-auth__aside-list">
          <li>{{ t('auth.resetAside1') }}</li>
          <li>{{ t('auth.resetAside2') }}</li>
        </ul>
      </div>

      <div class="pb-auth__links">
        <RouterLink to="/login">{{ t('auth.toLogin') }}</RouterLink>
      </div>
    </div>
  </section>
</template>
