<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { type AppLocale, SUPPORTED_LOCALES } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notifications'
import { usePreferencesStore } from '@/stores/preferences'

const { t } = useI18n()
const router = useRouter()
const preferences = usePreferencesStore()
const auth = useAuthStore()
const notifications = useNotificationStore()

/** progress 是文档滚动进度，顶栏底部用一条细线表达当前位置。 */
const progress = ref(0)

function updateProgress(): void {
  const root = document.documentElement
  const scrollable = root.scrollHeight - window.innerHeight
  progress.value = scrollable > 0 ? Math.min(1, Math.max(0, window.scrollY / scrollable)) : 0
}

function onScroll(): void {
  updateProgress()
}

/**
 * 未读数在登录态确定后拉取。
 *
 * 登出时必须清空：角标属于上一个身份，留着会让下一个访客看到别人的未读数。
 */
async function refreshUnread(): Promise<void> {
  if (auth.status !== 'authenticated') {
    notifications.clear()
    return
  }
  try {
    await notifications.refresh()
  } catch {
    // 角标读取失败不影响导航，保留上一次的值。
  }
}

watch(() => auth.status, refreshUnread)
onMounted(() => {
  void refreshUnread()
  updateProgress()
  window.addEventListener('scroll', onScroll, { passive: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
})

const localeOptions = computed(() =>
  SUPPORTED_LOCALES.map((value) => ({
    value,
    label: value === 'zh' ? '中文' : 'English',
  })),
)

function onLocaleChange(value: AppLocale): void {
  preferences.setLocale(value)
}

async function onLogout(): Promise<void> {
  await auth.logout()
  await router.push('/')
}
</script>

<template>
  <header class="pb-header">
    <div class="pb-container pb-header__inner">
      <RouterLink class="pb-brand" to="/">
        <span class="pb-brand__mark" aria-hidden="true">P</span>
        <span class="pb-brand__title">Prompt Benchmark</span>
        <span class="pb-brand__tagline">{{ t('app.tagline') }}</span>
      </RouterLink>

      <nav class="pb-header__actions">
        <div class="pb-header__links">
          <RouterLink class="pb-header__link" to="/" data-testid="header-home">
            {{ t('nav.home') }}
          </RouterLink>
          <RouterLink
            v-if="auth.status === 'authenticated'"
            class="pb-header__link"
            to="/upload"
            data-testid="header-upload"
          >
            {{ t('nav.upload') }}
          </RouterLink>
          <template v-if="auth.status === 'authenticated'">
            <span class="pb-header__sep" aria-hidden="true" />
            <RouterLink class="pb-header__link" to="/libraries" data-testid="header-libraries">
              {{ t('library.title') }}
            </RouterLink>
            <RouterLink class="pb-header__link" to="/favorites" data-testid="header-favorites">
              {{ t('prompt.favoriteManage') }}
            </RouterLink>
            <RouterLink
              v-if="auth.hasRole('admin')"
              class="pb-header__link"
              to="/admin"
              data-testid="header-admin"
            >
              {{ t('admin.entry') }}
            </RouterLink>
            <RouterLink
              class="pb-header__link"
              to="/notifications"
              data-testid="header-notifications"
            >
              {{ t('notifications.title') }}
              <span v-if="notifications.hasUnread" class="pb-header__badge" data-testid="header-unread">
                {{ notifications.unread }}
              </span>
            </RouterLink>
          </template>
        </div>

        <div class="pb-header__utils">
          <el-select
            class="pb-header__locale"
            :model-value="preferences.locale"
            :aria-label="t('nav.language')"
            size="small"
            @change="onLocaleChange"
          >
            <el-option
              v-for="option in localeOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>

          <button
            type="button"
            class="pb-header__icon"
            :aria-label="t('nav.theme')"
            :title="preferences.isDark ? t('nav.themeLight') : t('nav.themeDark')"
            @click="preferences.toggleTheme()"
          >
            <svg
              v-if="!preferences.isDark"
              viewBox="0 0 24 24"
              width="16"
              height="16"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              aria-hidden="true"
            >
              <circle cx="12" cy="12" r="4" />
              <path
                d="M12 2.5v2.2M12 19.3v2.2M2.5 12h2.2M19.3 12h2.2M5.2 5.2l1.6 1.6M17.2 17.2l1.6 1.6M18.8 5.2l-1.6 1.6M6.8 17.2l-1.6 1.6"
              />
            </svg>
            <svg
              v-else
              viewBox="0 0 24 24"
              width="16"
              height="16"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M20.5 14.2A8.5 8.5 0 0 1 9.8 3.5a8.5 8.5 0 1 0 10.7 10.7Z" />
            </svg>
            <span class="pb-header__icon-label">
              {{ preferences.isDark ? t('nav.themeLight') : t('nav.themeDark') }}
            </span>
          </button>

          <template v-if="auth.status === 'authenticated'">
            <el-dropdown trigger="click">
              <button type="button" class="pb-header__user" data-testid="header-user">
                {{ auth.user?.username }}
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item data-testid="header-profile" @click="router.push('/me')">
                    {{ t('nav.profile') }}
                  </el-dropdown-item>
                  <el-dropdown-item data-testid="header-logout" @click="onLogout">
                    {{ t('nav.logout') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
          <RouterLink v-else-if="auth.status === 'guest'" to="/login">
            <el-button size="small" type="primary" data-testid="header-login">
              {{ t('nav.login') }}
            </el-button>
          </RouterLink>
        </div>
      </nav>
    </div>
    <span
      class="pb-header__progress"
      :style="{ transform: `scaleX(${progress})` }"
      aria-hidden="true"
    />
  </header>
</template>

<style scoped>
.pb-header {
  position: sticky;
  top: 0;
  z-index: 30;
  background: var(--pb-header-bg);
  border-bottom: 1px solid var(--pb-line);
  backdrop-filter: blur(18px) saturate(1.35);
  -webkit-backdrop-filter: blur(18px) saturate(1.35);
}

/* 顶栏底部的渐变细线与滚动进度线共用一条位置，保持视觉整洁。 */
.pb-header::after {
  content: "";
  position: absolute;
  left: 0;
  right: 0;
  bottom: -1px;
  height: 1px;
  background: linear-gradient(
    90deg,
    transparent,
    var(--pb-accent) 22%,
    var(--pb-purpose) 62%,
    transparent
  );
  opacity: 0.28;
  pointer-events: none;
}

.pb-header__progress {
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 100%;
  height: 1.5px;
  transform-origin: left center;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose));
  opacity: 0.72;
  pointer-events: none;
  will-change: transform;
}

.pb-header__inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  height: var(--pb-header-height);
}

.pb-brand {
  display: flex;
  align-items: center;
  gap: var(--pb-space-sm);
  min-width: 0;
  color: var(--pb-ink);
}

.pb-brand__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  border-radius: 9px;
  background-color: var(--pb-ink);
  background-image: linear-gradient(135deg, var(--pb-ink) 0%, #3a332b 100%);
  color: var(--pb-paper);
  font-family: var(--pb-font-display);
  font-size: 17px;
  font-weight: 500;
  line-height: 1;
  box-shadow:
    0 5px 14px -8px rgba(26, 24, 21, 0.7),
    var(--pb-shadow-inset);
}

html.dark .pb-brand__mark {
  background-color: #f3f1ec;
  background-image: linear-gradient(135deg, #f3f1ec, #cfc8bc);
  color: #141415;
}

.pb-brand__title {
  font-family: var(--pb-font-display);
  font-size: 21px;
  font-weight: 500;
  letter-spacing: -0.015em;
  white-space: nowrap;
}

.pb-brand__tagline {
  color: var(--pb-ink-faint);
  font-size: 10.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pb-brand__tagline::before {
  content: "";
  display: inline-block;
  width: 3px;
  height: 3px;
  margin-right: 8px;
  border-radius: 50%;
  background: var(--pb-accent);
  vertical-align: middle;
}

.pb-header__actions {
  display: flex;
  align-items: center;
  gap: var(--pb-space-lg);
  min-width: 0;
}

.pb-header__links {
  display: flex;
  align-items: center;
  gap: var(--pb-space-md);
}

.pb-header__sep {
  width: 1px;
  height: 14px;
  background: var(--pb-line-strong);
}

.pb-header__link {
  position: relative;
  padding: 6px 1px;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  letter-spacing: 0.01em;
  white-space: nowrap;
}

.pb-header__link::after {
  content: "";
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 1.5px;
  border-radius: 2px;
  background: var(--pb-accent);
  transform: scaleX(0);
  transform-origin: left;
  transition: transform var(--pb-duration) var(--pb-ease);
}

.pb-header__link:hover {
  color: var(--pb-ink);
}

.pb-header__link:hover::after,
.pb-header__link.router-link-exact-active::after {
  transform: scaleX(1);
}

.pb-header__link.router-link-exact-active {
  color: var(--pb-ink);
}

.pb-header__link.router-link-active:not(.router-link-exact-active):not([href="/"]) {
  color: var(--pb-ink);
}

.pb-header__utils {
  display: flex;
  align-items: center;
  gap: var(--pb-space-sm);
}

.pb-header__locale {
  width: 84px;
}

.pb-header__locale :deep(.el-select__wrapper) {
  box-shadow: none;
  background: transparent;
  padding-left: 6px;
  padding-right: 6px;
}

.pb-header__locale :deep(.el-select__wrapper:hover) {
  background: var(--pb-surface-muted);
  box-shadow: none;
}

.pb-header__locale :deep(.el-select__wrapper.is-focused) {
  background: var(--pb-surface);
  box-shadow: 0 0 0 1px var(--pb-accent) inset;
}

.pb-header__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 32px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--pb-radius-pill);
  background: transparent;
  color: var(--pb-ink-soft);
  font-family: inherit;
  font-size: 12.5px;
  cursor: pointer;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    color var(--pb-duration) var(--pb-ease);
}

.pb-header__icon:hover {
  background: var(--pb-surface-muted);
  color: var(--pb-ink);
}

.pb-header__icon-label {
  line-height: 1;
}

.pb-header__user {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  max-width: 180px;
  padding: 6px 13px;
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  background: var(--pb-surface);
  color: var(--pb-ink);
  font-family: inherit;
  font-size: 13px;
  cursor: pointer;
  transition:
    border-color var(--pb-duration) var(--pb-ease),
    background-color var(--pb-duration) var(--pb-ease);
}

.pb-header__user::before {
  content: "";
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--pb-accent);
  flex-shrink: 0;
}

.pb-header__user:hover {
  border-color: var(--pb-line-strong);
  background: var(--pb-surface-muted);
}

.pb-header__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 17px;
  height: 17px;
  margin-left: 5px;
  padding: 0 5px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
  font-size: 10.5px;
  font-weight: 600;
  line-height: 1;
  vertical-align: 1px;
}

@media (max-width: 1080px) {
  .pb-brand__tagline {
    display: none;
  }
}

@media (max-width: 760px) {
  .pb-header {
    height: auto;
    min-height: var(--pb-header-height);
  }

  .pb-header__inner {
    flex-wrap: wrap;
    padding-top: var(--pb-space-sm);
    padding-bottom: var(--pb-space-sm);
    row-gap: var(--pb-space-sm);
  }

  .pb-header__actions {
    width: 100%;
    justify-content: space-between;
    gap: var(--pb-space-sm);
  }

  .pb-header__links {
    overflow-x: auto;
    padding-bottom: 2px;
  }

  .pb-header__link {
    font-size: 13px;
  }

  .pb-header__locale {
    width: 76px;
  }
}
</style>
