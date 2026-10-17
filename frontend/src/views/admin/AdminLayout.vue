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

/** NAV_ITEMS 是后台模块的导航顺序，与 C-42 的模块清单一致。 */
const NAV_ITEMS: AdminNavItem[] = [
  { name: 'admin-dashboard', path: '/admin/dashboard', labelKey: 'admin.nav.dashboard' },
  { name: 'admin-prompts', path: '/admin/prompts', labelKey: 'admin.nav.prompts' },
  { name: 'admin-models', path: '/admin/models', labelKey: 'admin.nav.models' },
  { name: 'admin-users', path: '/admin/users', labelKey: 'admin.nav.users' },
  { name: 'admin-tags', path: '/admin/tags', labelKey: 'admin.nav.tags' },
  { name: 'admin-dictionary', path: '/admin/dictionary', labelKey: 'admin.nav.dictionary' },
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
      <div class="pb-admin__heading">
        <span class="pb-eyebrow" aria-hidden="true">Administration</span>
        <h1 class="pb-admin__title" data-testid="admin-title">{{ t('admin.title') }}</h1>
      </div>
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
        <RouterView v-slot="{ Component }">
          <Transition name="pb-page" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </div>
    </div>
  </section>
</template>

<style scoped>
.pb-admin {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding-top: var(--pb-space-lg);
}

.pb-admin__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-admin__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
}

.pb-admin__title {
  font-size: 28px;
  font-weight: 600;
  letter-spacing: 0.012em;
  margin: 0;
}

.pb-admin__operator {
  padding: 3px 11px;
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-admin__denied {
  color: var(--pb-danger);
  margin: 0;
}

.pb-admin__layout {
  display: grid;
  grid-template-columns: 208px minmax(0, 1fr);
  gap: var(--pb-space-xl);
  align-items: start;
}

.pb-admin__nav {
  position: sticky;
  top: calc(var(--pb-header-height) + var(--pb-space-lg));
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--pb-space-sm);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
}

.pb-admin__nav-link {
  padding: 8px 12px;
  border-radius: var(--pb-radius-sm);
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    color var(--pb-duration) var(--pb-ease);
}

.pb-admin__nav-link:hover {
  background: var(--pb-surface-muted);
  color: var(--pb-ink);
}

.pb-admin__nav-link--active {
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
  font-weight: 600;
}

.pb-admin__content {
  min-width: 0;
}

@media (max-width: 900px) {
  .pb-admin__layout {
    grid-template-columns: 1fr;
    gap: var(--pb-space-lg);
  }

  .pb-admin__nav {
    position: static;
    flex-direction: row;
    flex-wrap: wrap;
    gap: var(--pb-space-xs);
  }

  .pb-admin__nav-link {
    padding: 6px 10px;
    font-size: 13px;
  }
}
</style>
