<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const year = new Date().getFullYear()
</script>

<template>
  <footer class="pb-footer">
    <div class="pb-container pb-footer__inner">
      <span class="pb-footer__copy">{{ t('footer.copyright') }} © {{ year }}</span>
      <nav class="pb-footer__links">
        <RouterLink class="pb-footer__link" to="/privacy" data-testid="footer-privacy">
          {{ t('footer.privacy') }}
        </RouterLink>
        <RouterLink class="pb-footer__link" to="/terms" data-testid="footer-terms">
          {{ t('footer.terms') }}
        </RouterLink>
      </nav>
    </div>
  </footer>
</template>

<style scoped>
.pb-footer {
  border-top: 1px solid var(--pb-color-border);
  background-color: var(--pb-color-bg-soft);
  color: var(--pb-color-text-soft);
  font-size: 13px;
}

.pb-footer__inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-md);
  flex-wrap: wrap;
  min-height: 52px;
  padding-top: var(--pb-space-sm);
  padding-bottom: var(--pb-space-sm);
}

.pb-footer__links {
  display: flex;
  gap: var(--pb-space-md);
}

.pb-footer__link:hover {
  color: var(--pb-color-primary);
}
</style>