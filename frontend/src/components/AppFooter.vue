<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const year = new Date().getFullYear()
</script>

<template>
  <footer class="pb-footer">
    <div class="pb-container pb-footer__inner">
      <div class="pb-footer__brand">
        <span class="pb-footer__mark">Prompt Benchmark</span>
        <span class="pb-footer__copy">{{ t('footer.copyright') }} © {{ year }}</span>
      </div>
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
  border-top: 1px solid var(--pb-line);
  background: transparent;
  color: var(--pb-ink-faint);
  font-size: 12.5px;
}

.pb-footer__inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-md);
  flex-wrap: wrap;
  padding-top: var(--pb-space-lg);
  padding-bottom: var(--pb-space-lg);
}

.pb-footer__brand {
  display: flex;
  align-items: baseline;
  gap: var(--pb-space-sm);
}

.pb-footer__mark {
  font-family: var(--pb-font-display);
  font-size: 14px;
  color: var(--pb-ink-soft);
  letter-spacing: -0.01em;
}

.pb-footer__copy {
  letter-spacing: 0.02em;
}

.pb-footer__links {
  display: flex;
  gap: var(--pb-space-lg);
}

.pb-footer__link {
  color: var(--pb-ink-faint);
  letter-spacing: 0.02em;
}

.pb-footer__link:hover {
  color: var(--pb-accent);
}
</style>
