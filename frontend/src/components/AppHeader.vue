<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
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
onMounted(refreshUnread)

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
        <span class="pb-brand__title">{{ t('app.title') }}</span>
        <span class="pb-brand__tagline">{{ t('app.tagline') }}</span>
      </RouterLink>

      <nav class="pb-header__actions">
        <RouterLink class="pb-header__link" to="/">
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
        <el-button size="small" :aria-label="t('nav.theme')" @click="preferences.toggleTheme()">
          {{ preferences.isDark ? t('nav.themeLight') : t('nav.themeDark') }}
        </el-button>

        <template v-if="auth.status === 'authenticated'">
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
          <RouterLink class="pb-header__link" to="/notifications" data-testid="header-notifications">
            {{ t('notifications.title') }}
            <span v-if="notifications.hasUnread" class="pb-header__badge" data-testid="header-unread">
              {{ notifications.unread }}
            </span>
          </RouterLink>
          <el-dropdown trigger="click">
            <el-button size="small" data-testid="header-user" class="pb-header__user">
              <span class="pb-header__user-name">{{ auth.user?.username }}</span>
            </el-button>
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
      </nav>
    </div>
  </header>
</template>

<style scoped>
.pb-header {
  position: sticky;
  top: 0;
  z-index: 10;
  height: var(--pb-header-height);
  background-color: var(--pb-color-surface);
  border-bottom: 1px solid var(--pb-color-border);
}

.pb-header__inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  gap: var(--pb-space-md);
}

.pb-brand {
  display: flex;
  align-items: baseline;
  gap: var(--pb-space-sm);
  min-width: 0;
}

.pb-brand__title {
  font-size: 18px;
  font-weight: 600;
  white-space: nowrap;
}

.pb-brand__tagline {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pb-header__actions {
  display: flex;
  align-items: center;
  gap: var(--pb-space-sm);
}

.pb-header__link {
  color: var(--pb-color-text-soft);
  font-size: 14px;
  padding: 0 var(--pb-space-sm);
}

.pb-header__link:hover {
  color: var(--pb-color-primary);
}

.pb-header__locale {
  width: 108px;
}

.pb-header__badge {
  background-color: var(--el-color-danger);
  border-radius: 8px;
  color: #fff;
  font-size: 11px;
  margin-left: 4px;
  padding: 0 5px;
}

/* 昵称最长 32 个字符，超长时省略以免把顶栏撑出视口。 */
.pb-header__user-name {
  display: inline-block;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: bottom;
  white-space: nowrap;
}

/*
 * 窄屏（375px 起）：顶栏不再固定 60px 高度，导航项折行排布。
 * 顶栏固定高度与不折行的横排无法同时容纳品牌、导航与账户入口，
 * 折行是唯一不隐藏任何入口的排布方式。
 */
@media (max-width: 640px) {
  .pb-header {
    height: auto;
    min-height: var(--pb-header-height);
  }

  .pb-header__inner {
    flex-wrap: wrap;
    row-gap: var(--pb-space-sm);
    padding-top: var(--pb-space-sm);
    padding-bottom: var(--pb-space-sm);
  }

  .pb-brand__tagline {
    display: none;
  }

  .pb-header__actions {
    flex-wrap: wrap;
    row-gap: var(--pb-space-sm);
  }

  .pb-header__locale {
    width: 92px;
  }

  .pb-header__user-name {
    max-width: 96px;
  }
}
</style>
