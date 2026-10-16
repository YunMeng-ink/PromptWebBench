// 私有题库域的接口封装：题库、成员、题目、标签、排序、回收站与导出。
// 类型与后端 OpenAPI 契约一一对应。

import { api } from './client'
import type { Page } from './prompt'

/** LibraryRole 是题库角色（需求 §10.3）。 */
export type LibraryRole = 'owner' | 'collaborator'

/** QuestionKind 是题目的两种来源（需求 §10.7）。 */
export type QuestionKind = 'reference' | 'derived'

export interface Library {
  id: number
  owner_id: number
  owner_name?: string
  name: string
  description: string
  role: LibraryRole
  member_count: number
  question_count: number
  live_question_count: number
  question_limit: number
  created_at: string
  updated_at: string
}

export interface LibraryMember {
  user_id: number
  username: string
  avatar_url: string
  role: LibraryRole
  invited_by?: number
  created_at: string
}

export interface LibraryQuestion {
  id: number
  library_id: number
  kind: QuestionKind
  title: string
  body: string
  description: string
  category_id?: number
  category_name_zh: string
  category_name_en: string
  ability_id?: number
  difficulty_id?: number
  sensitive_level: string
  sort_order: number
  tags: string[]
  source_prompt_id?: number
  source_version_no?: string
  /** source_available 为假表示来源提示词已被删除，题目没有正文可读。 */
  source_available: boolean
  source_major_modified: boolean
  created_at: string
  updated_at: string
}

export interface QuestionRevision {
  id: number
  editor_id: number
  editor_name: string
  note: string
  before: string
  after: string
  created_at: string
}

export interface RecycledQuestion {
  question: LibraryQuestion
  deleted_by: number
  deleted_at: string
  expires_at: string
}

export interface RecycledLibrary {
  library_id: number
  name: string
  description: string
  deleted_by: number
  deleted_at: string
  expires_at: string
}

export interface AddQuestionPayload {
  /** 引用题填来源提示词标识，派生题填标题与正文，两者互斥。 */
  prompt_id?: number
  title?: string
  body?: string
  description?: string
  category_id?: number
  ability_id?: number
  difficulty_id?: number
  sensitive_level?: string
  tags?: string[]
}

export interface UpdateQuestionPayload {
  title: string
  body: string
  description?: string
  category_id?: number
  ability_id?: number
  difficulty_id?: number
  sensitive_level?: string
  change_note?: string
}

export interface LibraryExport {
  filename: string
  content: string
}

export const libraryApi = {
  mine(page = 1, pageSize = 20): Promise<Page<Library>> {
    return api.get<Page<Library>>(`/me/libraries?page=${page}&page_size=${pageSize}`)
  },

  recycleBin(): Promise<{ items: RecycledLibrary[] }> {
    return api.get<{ items: RecycledLibrary[] }>('/me/libraries/recycle-bin')
  },

  create(name: string, description = ''): Promise<{ library: Library }> {
    return api.post<{ library: Library }>('/libraries', { name, description })
  },

  detail(id: number): Promise<{ library: Library }> {
    return api.get<{ library: Library }>(`/libraries/${id}`)
  },

  update(
    id: number,
    patch: { name?: string; description?: string },
  ): Promise<{ library: Library }> {
    return api.patch<{ library: Library }>(`/libraries/${id}`, patch)
  },

  remove(id: number): Promise<void> {
    return api.delete<void>(`/libraries/${id}`)
  },

  restore(id: number): Promise<{ library: Library }> {
    return api.post<{ library: Library }>(`/libraries/${id}/restore`, undefined)
  },

  purge(id: number): Promise<void> {
    return api.delete<void>(`/libraries/${id}/purge`)
  },

  export(id: number): Promise<LibraryExport> {
    return api.get<LibraryExport>(`/libraries/${id}/export`)
  },

  members(id: number): Promise<{ items: LibraryMember[] }> {
    return api.get<{ items: LibraryMember[] }>(`/libraries/${id}/members`)
  },

  invite(id: number, username: string): Promise<{ member: LibraryMember }> {
    return api.post<{ member: LibraryMember }>(`/libraries/${id}/members`, { username })
  },

  removeMember(id: number, userId: number): Promise<void> {
    return api.delete<void>(`/libraries/${id}/members/${userId}`)
  },

  questions(id: number, page = 1, pageSize = 50): Promise<Page<LibraryQuestion>> {
    return api.get<Page<LibraryQuestion>>(
      `/libraries/${id}/questions?page=${page}&page_size=${pageSize}`,
    )
  },

  addQuestion(id: number, payload: AddQuestionPayload): Promise<{ question: LibraryQuestion }> {
    return api.post<{ question: LibraryQuestion }>(`/libraries/${id}/questions`, payload)
  },

  reorder(id: number, ids: number[]): Promise<void> {
    return api.put<void>(`/libraries/${id}/questions/order`, { ids })
  },

  recycleQuestions(id: number): Promise<{ items: RecycledQuestion[] }> {
    return api.get<{ items: RecycledQuestion[] }>(`/libraries/${id}/recycle-bin`)
  },

  question(id: number): Promise<{ question: LibraryQuestion }> {
    return api.get<{ question: LibraryQuestion }>(`/library-questions/${id}`)
  },

  updateQuestion(
    id: number,
    payload: UpdateQuestionPayload,
  ): Promise<{ question: LibraryQuestion }> {
    return api.patch<{ question: LibraryQuestion }>(`/library-questions/${id}`, payload)
  },

  removeQuestion(id: number): Promise<void> {
    return api.delete<void>(`/library-questions/${id}`)
  },

  restoreQuestion(id: number): Promise<{ question: LibraryQuestion }> {
    return api.post<{ question: LibraryQuestion }>(`/library-questions/${id}/restore`, undefined)
  },

  purgeQuestion(id: number): Promise<void> {
    return api.delete<void>(`/library-questions/${id}/purge`)
  },

  addTag(id: number, tag: string): Promise<void> {
    return api.put<void>(`/library-questions/${id}/tags`, { tag })
  },

  removeTag(id: number, tag: string): Promise<void> {
    return api.delete<void>(`/library-questions/${id}/tags?tag=${encodeURIComponent(tag)}`)
  },

  revisions(id: number): Promise<{ items: QuestionRevision[] }> {
    return api.get<{ items: QuestionRevision[] }>(`/library-questions/${id}/revisions`)
  },
}

/**
 * libraryCapabilities 按角色给出可执行的操作，与后端 domain.LibraryActor 的两份清单一致
 * （需求 §10.5，C-46）。界面据此隐藏按钮，权限的真正判定仍在后端。
 */
export interface LibraryCapabilities {
  viewLibrary: boolean
  updateName: boolean
  updateDescription: boolean
  deleteLibrary: boolean
  inviteMember: boolean
  removeMember: boolean
  addQuestion: boolean
  deleteQuestion: boolean
  updateQuestion: boolean
  tagQuestion: boolean
  reorderQuestion: boolean
  exportLibrary: boolean
  restoreQuestion: boolean
  purgeQuestion: boolean
}

/** capabilitiesFor 返回给定的题库角色在界面上的可用操作。 */
export function capabilitiesFor(role: LibraryRole): LibraryCapabilities {
  if (role === 'owner') {
    return {
      viewLibrary: true,
      updateName: true,
      updateDescription: true,
      deleteLibrary: true,
      inviteMember: true,
      removeMember: true,
      addQuestion: true,
      deleteQuestion: true,
      updateQuestion: true,
      tagQuestion: true,
      reorderQuestion: true,
      exportLibrary: true,
      restoreQuestion: true,
      purgeQuestion: true,
    }
  }
  return {
    viewLibrary: true,
    updateName: false,
    updateDescription: true,
    deleteLibrary: false,
    inviteMember: false,
    removeMember: false,
    addQuestion: true,
    deleteQuestion: true,
    updateQuestion: true,
    tagQuestion: true,
    reorderQuestion: true,
    exportLibrary: true,
    restoreQuestion: true,
    purgeQuestion: false,
  }
}
