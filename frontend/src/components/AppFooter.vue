<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const year = new Date().getFullYear()
</script>

<template>
  <footer class="pb-footer">
    <div class="pb-container pb-footer__inner">
      <div class="pb-footer__brand">
        <RouterLink class="pb-footer__mark" to="/">Prompt Benchmark</RouterLink>
        <p class="pb-footer__note">{{ t('footer.note') }}</p>
        <span class="pb-footer__copy">{{ t('footer.copyright') }} © {{ year }}</span>
      </div>

      <nav class="pb-footer__links" :aria-label="t('footer.navLabel')">
        <div class="pb-footer__group">
          <span class="pb-footer__label">{{ t('footer.explore') }}</span>
          <RouterLink class="pb-footer__link" to="/">{{ t('nav.home') }}</RouterLink>
          <RouterLink class="pb-footer__link" to="/methodology" data-testid="footer-methodology">
            {{ t('footer.methodology') }}
          </RouterLink>
        </div>
        <div class="pb-footer__group">
          <span class="pb-footer__label">{{ t('footer.legal') }}</span>
          <RouterLink class="pb-footer__link" to="/privacy" data-testid="footer-privacy">
            {{ t('footer.privacy') }}
          </RouterLink>
          <RouterLink class="pb-footer__link" to="/terms" data-testid="footer-terms">
            {{ t('footer.terms') }}
          </RouterLink>
        </div>
      </nav>

      <div class="pb-footer__axes">
        <span class="pb-footer__axis">
          <span class="pb-axis-mark pb-axis-mark--purpose" aria-hidden="true" />
          {{ t('dimension.purpose') }}
        </span>
        <span class="pb-footer__axis">
          <span class="pb-axis-mark pb-axis-mark--cognitive" aria-hidden="true" />
          {{ t('dimension.groupCognitive') }}
        </span>
        <span class="pb-footer__axis">
          <span class="pb-axis-mark pb-axis-mark--execution" aria-hidden="true" />
          {{ t('dimension.groupExecution') }}
        </span>
        <span class="pb-footer__axis">
          <span class="pb-axis-mark pb-axis-mark--reliability" aria-hidden="true" />
          {{ t('dimension.groupReliability') }}
        </span>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.pb-footer {
  position: relative;
  border-top: 1px solid var(--pb-line);
  background: color-mix(in srgb, var(--pb-surface-muted) 62%, transparent);
  color: var(--pb-ink-faint);
  font-size: 12.5px;
}

.pb-footer::before {
  content: "";
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 1px;
  background: linear-gradient(90deg, transparent, var(--pb-purpose), var(--pb-accent), transparent);
  opacity: 0.34;
}

.pb-footer__inner {
  display: grid;
  grid-template-columns: minmax(220px, 1.4fr) minmax(240px, 1fr) auto;
  align-items: start;
  gap: var(--pb-space-xl);
  padding-top: var(--pb-space-xl);
  padding-bottom: var(--pb-space-xl);
}

.pb-footer__brand {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  min-width: 0;
}

.pb-footer__mark {
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 17px;
  letter-spacing: -0.01em;
}

.pb-footer__mark:hover {
  color: var(--pb-accent);
}

.pb-footer__note {
  max-width: 34ch;
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 12.5px;
  line-height: 1.7;
}

.pb-footer__copy {
  letter-spacing: 0.02em;
}

.pb-footer__links {
  display: flex;
  gap: var(--pb-space-2xl);
}

.pb-footer__group {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.pb-footer__label {
  margin-bottom: 3px;
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.pb-footer__link {
  width: fit-content;
  color: var(--pb-ink-soft);
  letter-spacing: 0.02em;
  transition:
    color var(--pb-duration) var(--pb-ease),
    transform var(--pb-duration) var(--pb-ease);
}

.pb-footer__link:hover {
  color: var(--pb-accent);
  transform: translateX(2px);
}

.pb-footer__axes {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--pb-space-xs) var(--pb-space-md);
  padding-top: var(--pb-space-xs);
}

.pb-footer__axis {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .pb-footer__inner {
    grid-template-columns: 1fr 1fr;
  }

  .pb-footer__axes {
    grid-column: 1 / -1;
    justify-content: flex-start;
    padding-top: 0;
  }
}

@media (max-width: 640px) {
  .pb-footer__inner {
    grid-template-columns: 1fr;
    gap: var(--pb-space-lg);
  }

  .pb-footer__links {
    gap: var(--pb-space-2xl);
  }
}
</style>
