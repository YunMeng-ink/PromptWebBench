// 社交与协作域的接口封装：收藏夹、关注、评论、举报、通知与公告。
// 类型与后端 OpenAPI 契约一一对应。

import { api } from './client'
import type { Page } from './prompt'

export type FavoriteVisibility = 'public' | 'private'

export interface Favorite {
  id: number
  owner_id: number
  owner_name?: string
  name: string
  description: string
  visibility: FavoriteVisibility
  item_count: number
  created_at: string
  updated_at: string
}

export interface FavoriteItem {
  prompt_id: number
  title: string
  description: string
  version_no: string
  visibility: string
  sensitive_level: string
  purpose_name_zh: string
  purpose_name_en: string
  capability_name_zh: string
  capability_name_en: string
  added_at: string
}

export interface Profile {
  id: number
  username: string
  bio: string
  avatar_url: string
  followed: boolean
}

export interface Comment {
  id: number
  prompt_id: number
  user_id: number
  author_name: string
  parent_id?: number
  body: string
  like_count: number
  liked: boolean
  reply_count: number
  created_at: string
}

/** ReportReason 是举报原因的英文枚举键（C-31），中文文案在前端语言包。 */
export type ReportReason =
  | 'bad_content'
  | 'illegal'
  | 'porn'
  | 'attack'
  | 'privacy'
  | 'spam'
  | 'dangerous'
  | 'other'

/** REPORT_REASONS 是举报原因的全部取值，顺序即界面展示顺序。 */
export const REPORT_REASONS: ReportReason[] = [
  'bad_content',
  'illegal',
  'porn',
  'attack',
  'privacy',
  'spam',
  'dangerous',
  'other',
]

export type ReportObjectType = 'prompt' | 'comment'

export interface ReportPayload {
  object_type: ReportObjectType
  object_id: number
  reason: ReportReason
  detail?: string
}

export type NotificationKind =
  | 'announcement'
  | 'comment_reply'
  | 'library_question_deleted'
  | 'system'

/** NOTIFICATION_KINDS 是通知类型筛选器的全部取值。 */
export const NOTIFICATION_KINDS: NotificationKind[] = [
  'announcement',
  'comment_reply',
  'library_question_deleted',
  'system',
]

export interface Notification {
  id: number
  kind: NotificationKind
  title: string
  body: string
  link: string
  read: boolean
  created_at: string
}

export interface Announcement {
  id: number
  title: string
  body: string
  published_at: string
  read: boolean
}

export interface FavoritePayload {
  name: string
  description?: string
  visibility?: FavoriteVisibility
}

export const socialApi = {
  // ---------- 收藏夹 ----------

  favoritesOf(userId: number, page = 1, pageSize = 20): Promise<Page<Favorite>> {
    return api.get<Page<Favorite>>(`/users/${userId}/favorites?page=${page}&page_size=${pageSize}`)
  },

  myFavorites(page = 1, pageSize = 20): Promise<Page<Favorite>> {
    return api.get<Page<Favorite>>(`/me/favorites?page=${page}&page_size=${pageSize}`)
  },

  favoriteItems(favoriteId: number, page = 1, pageSize = 20): Promise<Page<FavoriteItem>> {
    return api.get<Page<FavoriteItem>>(
      `/favorites/${favoriteId}/items?page=${page}&page_size=${pageSize}`,
    )
  },

  createFavorite(payload: FavoritePayload): Promise<{ favorite: Favorite }> {
    return api.post<{ favorite: Favorite }>('/favorites', payload)
  },

  updateFavorite(id: number, payload: FavoritePayload): Promise<{ favorite: Favorite }> {
    return api.patch<{ favorite: Favorite }>(`/favorites/${id}`, payload)
  },

  removeFavorite(id: number): Promise<void> {
    return api.delete<void>(`/favorites/${id}`)
  },

  addItem(favoriteId: number, promptId: number): Promise<void> {
    return api.put<void>(`/favorites/${favoriteId}/items/${promptId}`, undefined)
  },

  removeItem(favoriteId: number, promptId: number): Promise<void> {
    return api.delete<void>(`/favorites/${favoriteId}/items/${promptId}`)
  },

  isFavorited(promptId: number): Promise<{ favorited: boolean }> {
    return api.get<{ favorited: boolean }>(`/me/favorited/${promptId}`)
  },

  // ---------- 关注 ----------

  profile(userId: number): Promise<{ profile: Profile }> {
    return api.get<{ profile: Profile }>(`/users/${userId}/profile`)
  },

  following(page = 1, pageSize = 20): Promise<Page<Profile>> {
    return api.get<Page<Profile>>(`/me/following?page=${page}&page_size=${pageSize}`)
  },

  follow(userId: number): Promise<void> {
    return api.put<void>(`/me/following/${userId}`, undefined)
  },

  unfollow(userId: number): Promise<void> {
    return api.delete<void>(`/me/following/${userId}`)
  },

  // ---------- 评论 ----------

  comments(promptId: number, page = 1, pageSize = 20): Promise<Page<Comment>> {
    return api.get<Page<Comment>>(
      `/prompts/${promptId}/comments?page=${page}&page_size=${pageSize}`,
    )
  },

  replies(commentId: number, page = 1, pageSize = 20): Promise<Page<Comment>> {
    return api.get<Page<Comment>>(
      `/comments/${commentId}/replies?page=${page}&page_size=${pageSize}`,
    )
  },

  createComment(promptId: number, body: string, parentId?: number): Promise<{ comment: Comment }> {
    return api.post<{ comment: Comment }>(`/prompts/${promptId}/comments`, {
      body,
      parent_id: parentId,
    })
  },

  removeComment(commentId: number): Promise<void> {
    return api.delete<void>(`/comments/${commentId}`)
  },

  likeComment(commentId: number): Promise<void> {
    return api.put<void>(`/comments/${commentId}/like`, undefined)
  },

  unlikeComment(commentId: number): Promise<void> {
    return api.delete<void>(`/comments/${commentId}/like`)
  },

  // ---------- 举报 ----------

  report(payload: ReportPayload): Promise<{ id: number; reason: string }> {
    return api.post<{ id: number; reason: string }>('/reports', payload)
  },

  // ---------- 通知与公告 ----------

  notifications(
    kinds: NotificationKind[] = [],
    page = 1,
    pageSize = 20,
  ): Promise<Page<Notification>> {
    const search = new URLSearchParams()
    search.set('page', String(page))
    search.set('page_size', String(pageSize))
    for (const kind of kinds) {
      search.append('kind', kind)
    }
    return api.get<Page<Notification>>(`/me/notifications?${search.toString()}`)
  },

  unreadCount(): Promise<{ unread: number }> {
    return api.get<{ unread: number }>('/me/notifications/unread-count')
  },

  markRead(id: number): Promise<void> {
    return api.post<void>(`/me/notifications/${id}/read`, undefined)
  },

  markAllRead(): Promise<void> {
    return api.post<void>('/me/notifications/read-all', undefined)
  },

  removeNotification(id: number): Promise<void> {
    return api.delete<void>(`/me/notifications/${id}`)
  },

  announcements(page = 1, pageSize = 10): Promise<Page<Announcement>> {
    return api.get<Page<Announcement>>(`/announcements?page=${page}&page_size=${pageSize}`)
  },

  markAnnouncementsRead(): Promise<void> {
    return api.post<void>('/me/announcements/read-all', undefined)
  },
}
