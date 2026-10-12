// 通知与公告的共享状态。
//
// 未读数在顶栏常驻展示，因此需要跨视图共享一份并保持同步：
// 通知中心里的「标记已读」与「删除」都要立刻反映到顶栏的角标上。

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { socialApi } from '@/api/social'

export const useNotificationStore = defineStore('notifications', () => {
  const unread = ref(0)
  const loaded = ref(false)

  const hasUnread = computed(() => unread.value > 0)

  /** refresh 拉取未读数。未登录时由调用方决定是否调用。 */
  async function refresh(): Promise<void> {
    const result = await socialApi.unreadCount()
    unread.value = result.unread
    loaded.value = true
  }

  /** clear 在登出时清空，避免下一个身份看到上一个身份的角标。 */
  function clear(): void {
    unread.value = 0
    loaded.value = false
  }

  /** setUnread 由通知中心的批量操作直接写入，省一次往返。 */
  function setUnread(value: number): void {
    unread.value = Math.max(0, value)
  }

  return { unread, loaded, hasUnread, refresh, clear, setUnread }
})
