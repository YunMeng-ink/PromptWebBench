<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { gitHubAuthorizePath } from '@/api/auth'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const email = ref('')
const password = ref('')
const submitting = ref(false)
const errorText = ref('')

async function submit(): Promise<void> {
  submitting.value = true
  errorText.value = ''
  try {
    await auth.login(email.value, password.value)
    const target = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.push(target)
  } catch (error) {
    errorText.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="pb-auth">
    <h1 class="pb-auth__title">{{ t('auth.loginTitle') }}</h1>
    <p class="pb-auth__hint">{{ t('auth.loginHint') }}</p>

    <el-alert v-if="errorText" class="pb-auth__error" type="error" :title="errorText" show-icon />

    <el-form label-position="top" data-testid="login-form" @submit.prevent="submit">
      <el-form-item :label="t('auth.email')">
        <el-input v-model="email" type="email" autocomplete="email" data-testid="login-email" />
      </el-form-item>
      <el-form-item :label="t('auth.password')">
        <el-input
          v-model="password"
          type="password"
          autocomplete="current-password"
          show-password
          data-testid="login-password"
        />
      </el-form-item>
      <el-button
        type="primary"
        native-type="submit"
        :loading="submitting"
        data-testid="login-submit"
      >
        {{ t('auth.submitLogin') }}
      </el-button>
    </el-form>

    <div class="pb-auth__links">
      <RouterLink to="/reset-password">{{ t('auth.forgotPassword') }}</RouterLink>
      <RouterLink to="/register">{{ t('auth.toRegister') }}</RouterLink>
    </div>

    <el-divider />
    <a class="pb-auth__oauth" :href="gitHubAuthorizePath">{{ t('auth.useGitHub') }}</a>
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

.pb-auth__error {
  margin-bottom: var(--pb-space-md);
}

.pb-auth__links {
  display: flex;
  justify-content: space-between;
  margin-top: var(--pb-space-md);
  font-size: 14px;
}

.pb-auth__links a,
.pb-auth__oauth {
  color: var(--pb-color-primary);
}
</style>
