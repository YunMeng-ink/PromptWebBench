<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { gitHubAuthorizePath } from '@/api/auth'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const inviteCode = ref('')
const submitting = ref(false)
const errorText = ref('')

async function submit(): Promise<void> {
  errorText.value = ''

  if (password.value !== confirmPassword.value) {
    errorText.value = t('auth.passwordMismatch')
    return
  }

  submitting.value = true
  try {
    await auth.register(email.value, password.value, inviteCode.value)
    await router.push({ name: 'verify-email', query: { email: email.value } })
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="pb-auth">
    <div class="pb-auth__card">
      <span class="pb-auth__eyebrow" aria-hidden="true">Create Account</span>
      <h1 class="pb-auth__title">{{ t('auth.registerTitle') }}</h1>
      <p class="pb-auth__hint">{{ t('auth.registerHint') }}</p>

      <el-alert v-if="errorText" class="pb-auth__error" type="error" :title="errorText" show-icon />

      <el-form label-position="top" data-testid="register-form" @submit.prevent="submit">
        <el-form-item :label="t('auth.email')">
          <el-input v-model="email" type="email" autocomplete="email" data-testid="register-email" />
        </el-form-item>
        <el-form-item :label="t('auth.password')">
          <el-input
            v-model="password"
            type="password"
            autocomplete="new-password"
            show-password
            data-testid="register-password"
          />
        </el-form-item>
        <el-form-item :label="t('auth.confirmPassword')">
          <el-input
            v-model="confirmPassword"
            type="password"
            autocomplete="new-password"
            show-password
            data-testid="register-confirm"
          />
        </el-form-item>
        <el-form-item :label="t('auth.inviteCode')">
          <el-input v-model="inviteCode" data-testid="register-invite" />
        </el-form-item>
        <el-button
          class="pb-auth__submit"
          type="primary"
          native-type="submit"
          :loading="submitting"
          data-testid="register-submit"
        >
          {{ t('auth.submitRegister') }}
        </el-button>
      </el-form>

      <div class="pb-auth__links">
        <RouterLink to="/login">{{ t('auth.toLogin') }}</RouterLink>
      </div>

      <el-divider />
      <a class="pb-auth__oauth" :href="gitHubAuthorizePath">{{ t('auth.useGitHub') }}</a>
    </div>
  </section>
</template>
