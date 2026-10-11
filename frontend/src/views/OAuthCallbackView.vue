<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { translateErrorCode } from '@/i18n'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const failed = ref(false)
const errorText = ref('')

const errorCode = computed(() => (typeof route.query.error === 'string' ? route.query.error : ''))

onMounted(async () => {
  // 后端已在校验通过后下发会话 Cookie，这里只需确认登录态。
  await auth.fetchSession()
  if (auth.isAuthenticated) {
    await router.replace('/')
    return
  }

  failed.value = true
  errorText.value =
    errorCode.value === ''
      ? t('auth.oauthFailed')
      : translateErrorCode(errorCode.value, t('auth.oauthFailed'))
})
</script>

<template>
  <section class="pb-auth">
    <h1 class="pb-auth__title">{{ t('auth.oauthTitle') }}</h1>
    <el-alert v-if="failed" type="error" :title="errorText" show-icon data-testid="oauth-error" />
    <p v-else class="pb-auth__hint">{{ t('auth.oauthSuccess') }}</p>

    <div class="pb-auth__links">
      <RouterLink to="/login">{{ t('auth.toLogin') }}</RouterLink>
    </div>
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
  margin: 0 0 var(--pb-space-lg);
}

.pb-auth__hint {
  color: var(--pb-color-text-soft);
}

.pb-auth__links {
  margin-top: var(--pb-space-md);
  font-size: 14px;
}

.pb-auth__links a {
  color: var(--pb-color-primary);
}
</style>
