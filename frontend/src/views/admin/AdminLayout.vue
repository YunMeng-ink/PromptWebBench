// 管理后台的外框：左侧模块导航 + 右侧内容区。
// 导航用普通路由链接而不是折叠菜单：11 个模块在窄屏下换行展示，
// 不需要额外的展开状态，键盘也能直接到达每一项。

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const auth = useAuthStore()

interface AdminNavItem {
  name: string
  path: string
  labelKey: string
}

/** NAV_ITEMS 是 11 个模块的导航顺序，与 C-42 的模块清单一致。 */
const NAV_ITEMS: AdminNavItem[] = [
  { name: 'admin-dashboard', path: '/admin/dashboard', labelKey: 'admin.nav.dashboard' },
  { name: 'admin-prompts', path: '/admin/prompts', labelKey: 'admin.nav.prompts' },
  { name: 'admin-models', path: '/admin/models', labelKey: 'admin.nav.models' },
  { name: 'admin-users', path: '/admin/users', labelKey: 'admin.nav.users' },
  { name: 'admin-tags', path: '/admin/tags', labelKey: 'admin.nav.tags' },
  { name: 'admin-reports', path: '/admin/reports', labelKey: 'admin.nav.reports' },
  { name: 'admin-invites', path: '/admin/invites', labelKey: 'admin.nav.invites' },
  { name: 'admin-logs', path: '/admin/logs', labelKey: 'admin.nav.logs' },
  {
    name: 'admin-announcements',
    path: '/admin/announcements',
    labelKey: 'admin.nav.announcements',
  },
  { name: 'admin-recycle', path: '/admin/recycle', labelKey: 'admin.nav.recycle' },
  { name: 'admin-settings', path: '/admin/settings', labelKey: 'admin.nav.settings' },
]

const activePath = computed(() => `/admin/${String(route.name ?? '').replace('admin-', '')}`)
</script>

<template>
  <section class="pb-admin">
    <header class="pb-admin__head">
      <h1 class="pb-admin__title" data-testid="admin-title">{{ t('admin.title') }}</h1>
      <span class="pb-admin__operator" data-testid="admin-operator">{{ auth.user?.username }}</span>
    </header>

    <p v-if="!auth.hasRole('admin')" class="pb-admin__denied" data-testid="admin-denied">
      {{ t('admin.guardDenied') }}
    </p>

    <div v-else class="pb-admin__layout">
      <nav class="pb-admin__nav" data-testid="admin-nav">
        <RouterLink
          v-for="item in NAV_ITEMS"
          :key="item.name"
          class="pb-admin__nav-link"
          :class="{ 'pb-admin__nav-link--active': activePath === item.path }"
          :to="item.path"
          :data-testid="`admin-nav-${item.name}`"
        >
          {{ t(item.labelKey) }}
        </RouterLink>
      </nav>

      <div class="pb-admin__content">
        <RouterView />
      </div>
    </div>
  </section>
</template>

<style scoped>
.pb-admin {
  padding: var(--pb-space-lg) 0;
}

.pb-admin__head {
  align-items: baseline;
  display: flex;
  gap: var(--pb-space-md);
  justify-content: space-between;
}

.pb-admin__title {
  font-size: 22px;
  margin: 0;
}

.pb-admin__operator {
  color: var(--pb-color-text-faint);
  font-size: 13px;
}

.pb-admin__denied {
  color: var(--el-color-danger);
  margin-top: var(--pb-space-md);
}

.pb-admin__layout {
  display: grid;
  gap: var(--pb-space-lg);
  grid-template-columns: 190px 1fr;
  margin-top: var(--pb-space-md);
}

.pb-admin__nav {
  align-content: start;
  display: grid;
  gap: 2px;
}

.pb-admin__nav-link {
  border-radius: 4px;
  color: var(--pb-color-text-soft);
  font-size: 14px;
  padding: 6px var(--pb-space-sm);
}

.pb-admin__nav-link:hover {
  background-color: var(--pb-color-surface);
  color: var(--pb-color-primary);
}

.pb-admin__nav-link--active {
  background-color: var(--pb-color-surface);
  color: var(--pb-color-primary);
  font-weight: 600;
}

.pb-admin__content {
  min-width: 0;
}

@media (max-width: 900px) {
  .pb-admin__layout {
    grid-template-columns: 1fr;
  }

  .pb-admin__nav {
    display: flex;
    flex-wrap: wrap;
    gap: var(--pb-space-xs);
  }
}
</style>
