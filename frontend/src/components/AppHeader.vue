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

/** mobileMenuOpen 控制窄屏导航面板的展开状态。 */
const mobileMenuOpen = ref(false)

interface HeaderLink {
  key: string
  path: string
  labelKey: string
  testId: string
}

/**
 * primaryLinks 是宽屏顶栏直接展示的入口。
 * 数量控制在四个以内，其余入口收进「更多」菜单，避免导航挤压右侧工具区。
 */
const primaryLinks = computed<HeaderLink[]>(() => {
  const items: HeaderLink[] = [
    { key: 'home', path: '/', labelKey: 'nav.home', testId: 'header-home' },
    {
      key: 'leaderboard',
      path: '/leaderboard',
      labelKey: 'nav.leaderboard',
      testId: 'header-leaderboard',
    },
    {
      key: 'showcase',
      path: '/showcase',
      labelKey: 'nav.showcase',
      testId: 'header-showcase',
    },
  ]
  if (auth.status === 'authenticated') {
    items.push({ key: 'upload', path: '/upload', labelKey: 'nav.upload', testId: 'header-upload' })
  }
  return items
})

/** secondaryLinks 是收进「更多」菜单的次要入口，同样只对登录身份出现。 */
const secondaryLinks = computed<HeaderLink[]>(() => {
  if (auth.status !== 'authenticated') {
    return []
  }
  const items: HeaderLink[] = [
    {
      key: 'libraries',
      path: '/libraries',
      labelKey: 'library.title',
      testId: 'header-libraries',
    },
    {
      key: 'favorites',
      path: '/favorites',
      labelKey: 'prompt.favoriteManage',
      testId: 'header-favorites',
    },
  ]
  if (auth.hasRole('admin')) {
    items.push({ key: 'admin', path: '/admin', labelKey: 'admin.entry', testId: 'header-admin' })
  }
  return items
})

/** mobileLinks 是窄屏面板中的完整导航，顺序与宽屏一致。 */
const mobileLinks = computed<HeaderLink[]>(() => [...primaryLinks.value, ...secondaryLinks.value])

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

      <nav class="pb-header__actions" :aria-label="t('nav.navigation')">
        <div class="pb-header__links">
          <RouterLink
            v-for="item in primaryLinks"
            :key="item.key"
            class="pb-header__link"
            :to="item.path"
            :data-testid="item.testId"
          >
            {{ t(item.labelKey) }}
          </RouterLink>

          <el-dropdown v-if="secondaryLinks.length > 0" trigger="click" placement="bottom-end">
            <button
              type="button"
              class="pb-header__link pb-header__more-trigger"
              :aria-label="t('nav.more')"
              data-testid="header-more"
            >
              {{ t('nav.more') }}
              <svg
                viewBox="0 0 24 24"
                width="12"
                height="12"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path d="m6 9 6 6 6-6" />
              </svg>
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  v-for="item in secondaryLinks"
                  :key="item.key"
                  :data-testid="item.testId"
                  @click="router.push(item.path)"
                >
                  {{ t(item.labelKey) }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>

        <div class="pb-header__utils">
          <el-select
            class="pb-header__locale pb-header__desktop-only"
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
            class="pb-header__icon pb-header__desktop-only"
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

          <RouterLink
            v-if="auth.status === 'authenticated'"
            class="pb-header__icon pb-header__bell"
            :aria-label="t('notifications.title')"
            :title="t('notifications.title')"
            to="/notifications"
            data-testid="header-notifications"
          >
            <svg
              viewBox="0 0 24 24"
              width="17"
              height="17"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
              <path d="M13.7 21a2 2 0 0 1-3.4 0" />
            </svg>
            <span
              v-if="notifications.hasUnread"
              class="pb-header__badge pb-header__badge--bell"
              data-testid="header-unread"
            >
              {{ notifications.unread }}
            </span>
          </RouterLink>

          <el-popover
            v-model:visible="mobileMenuOpen"
            placement="bottom-end"
            :width="236"
            trigger="click"
            popper-class="pb-header__menu-popover"
          >
            <template #reference>
              <button
                type="button"
                class="pb-header__icon pb-header__mobile-menu"
                :aria-label="t('nav.menu')"
                :title="t('nav.menu')"
                data-testid="header-menu"
              >
                <svg
                  viewBox="0 0 24 24"
                  width="18"
                  height="18"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.7"
                  stroke-linecap="round"
                  aria-hidden="true"
                >
                  <path d="M4 7h16M4 12h16M4 17h16" />
                </svg>
              </button>
            </template>

            <nav class="pb-header__menu" data-testid="header-menu-panel">
              <RouterLink
                v-for="item in mobileLinks"
                :key="item.key"
                class="pb-header__menu-link"
                :to="item.path"
                :data-testid="`mobile-${item.testId}`"
                @click="mobileMenuOpen = false"
              >
                {{ t(item.labelKey) }}
              </RouterLink>

              <span class="pb-header__menu-divider" aria-hidden="true" />

              <el-select
                class="pb-header__menu-locale"
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
                class="pb-header__menu-action"
                @click="preferences.toggleTheme()"
              >
                {{ preferences.isDark ? t('nav.themeLight') : t('nav.themeDark') }}
              </button>
            </nav>
          </el-popover>

          <template v-if="auth.status === 'authenticated'">
            <el-dropdown trigger="click">
              <button
                type="button"
                class="pb-header__user"
                :title="auth.user?.username"
                data-testid="header-user"
              >
                <span class="pb-header__user-name">{{ auth.user?.username }}</span>
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
  overflow: hidden;
  font-family: var(--pb-font-display);
  font-size: 21px;
  font-weight: 500;
  letter-spacing: -0.015em;
  text-overflow: ellipsis;
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
  min-width: 0;
}

.pb-header__more-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  font-family: inherit;
  cursor: pointer;
}

.pb-header__more-trigger svg {
  transition: transform var(--pb-duration) var(--pb-ease);
}

.pb-header__more-trigger:hover svg {
  transform: translateY(1px);
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

.pb-header__bell {
  position: relative;
  text-decoration: none;
}

.pb-header__badge--bell {
  position: absolute;
  top: 1px;
  right: 1px;
  min-width: 16px;
  height: 16px;
  margin-left: 0;
  padding: 0 4px;
  border: 2px solid var(--pb-header-bg);
  font-size: 10px;
}

.pb-header__mobile-menu {
  display: none;
}

.pb-header__menu {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pb-header__menu-link {
  padding: 8px 10px;
  border-radius: var(--pb-radius-sm);
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    color var(--pb-duration) var(--pb-ease);
}

.pb-header__menu-link:hover,
.pb-header__menu-link.router-link-exact-active {
  background: var(--pb-surface-muted);
  color: var(--pb-ink);
}

.pb-header__menu-divider {
  height: 1px;
  margin: 6px 2px;
  background: var(--pb-line);
}

.pb-header__menu-locale {
  width: 100%;
}

.pb-header__menu-action {
  padding: 8px 10px;
  border: 0;
  border-radius: var(--pb-radius-sm);
  background: transparent;
  color: var(--pb-ink-soft);
  font-family: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}

.pb-header__menu-action:hover {
  background: var(--pb-surface-muted);
  color: var(--pb-ink);
}

.pb-header__user {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  max-width: 180px;
  min-width: 0;
  overflow: hidden;
  padding: 6px 13px;
  white-space: nowrap;
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

.pb-header__user-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

@media (max-width: 1360px) {
  .pb-brand__tagline {
    display: none;
  }
}

/* 中等宽度先收紧间距与次要文案，避免导航项和右侧工具挤在一起。 */
@media (max-width: 980px) {
  .pb-header__inner {
    gap: var(--pb-space-md);
  }

  .pb-header__actions {
    gap: var(--pb-space-md);
  }

  .pb-header__links {
    gap: var(--pb-space-sm);
  }

  .pb-header__icon-label {
    display: none;
  }

  .pb-header__locale {
    width: 72px;
  }
}

/* 窄屏改为单行：品牌与工具同排，导航全部收进菜单面板。 */
@media (max-width: 900px) {
  .pb-header {
    height: auto;
    min-height: var(--pb-header-height);
  }

  .pb-header__inner {
    display: grid;
    grid-template-areas: "brand utils";
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--pb-space-sm);
    height: auto;
    padding-bottom: var(--pb-space-sm);
    padding-top: var(--pb-space-sm);
  }

  .pb-header__actions {
    display: contents;
  }

  .pb-brand {
    grid-area: brand;
  }

  .pb-header__links {
    display: none;
  }

  .pb-header__desktop-only {
    display: none;
  }

  .pb-header__mobile-menu {
    display: inline-flex;
  }

  .pb-header__utils {
    grid-area: utils;
    justify-self: end;
    min-width: 0;
  }

  .pb-header__user {
    max-width: 112px;
  }
}

@media (max-width: 460px) {
  .pb-brand__title {
    font-size: 17px;
  }

  .pb-header__locale {
    width: 68px;
  }

  .pb-header__user {
    max-width: 84px;
    padding-left: 10px;
    padding-right: 10px;
  }
}
</style>
