// 字典数据的进程内缓存。考察目的、考察维度、难度、主题标签与模型变动极少，
// 每个页面各拉一次会造成重复请求，这里统一缓存并允许手动刷新。

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  type Capability,
  type Difficulty,
  dictionaryApi,
  type Model,
  type Purpose,
  type Tag,
} from '@/api/prompt'

interface Dictionaries {
  purposes: Purpose[]
  capabilities: Capability[]
  difficulties: Difficulty[]
  tags: Tag[]
  models: Model[]
}

const empty: Dictionaries = {
  purposes: [],
  capabilities: [],
  difficulties: [],
  tags: [],
  models: [],
}

export const useDictionaryStore = defineStore('dictionary', () => {
  const data = ref<Dictionaries>({ ...empty })
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref('')

  /** inFlight 让并发调用共享同一次请求，避免首屏重复拉取。 */
  let inFlight: Promise<void> | null = null

  async function load(force = false): Promise<void> {
    if (loaded.value && !force) {
      return
    }
    if (inFlight !== null) {
      return inFlight
    }

    loading.value = true
    error.value = ''
    inFlight = (async () => {
      try {
        const [purposes, capabilities, difficulties, tags, models] = await Promise.all([
          dictionaryApi.purposes(),
          dictionaryApi.capabilities(),
          dictionaryApi.difficulties(),
          dictionaryApi.tags(),
          dictionaryApi.models(),
        ])
        data.value = {
          purposes: purposes.items,
          capabilities: capabilities.items,
          difficulties: difficulties.items,
          tags: tags.items,
          models: models.items,
        }
        loaded.value = true
      } catch {
        error.value = 'dictionary_load_failed'
      } finally {
        loading.value = false
        inFlight = null
      }
    })()
    return inFlight
  }

  /**
   * refreshTags 只重新拉取主题标签。
   *
   * 标签可以在后台随时增删，而字典缓存在整个会话内有效；
   * 进入提示词表单时刷新标签，管理员刚创建的标签才能立即出现在下拉菜单中。
   * 接口带共享缓存头，这里显式绕过浏览器与 CDN 的副本。
   */
  async function refreshTags(): Promise<void> {
    try {
      const result = await dictionaryApi.tags({ cache: 'no-store' })
      data.value = { ...data.value, tags: result.items }
      error.value = ''
    } catch {
      error.value = 'dictionary_load_failed'
    }
  }

  function purposeById(id: number | undefined): Purpose | undefined {
    return data.value.purposes.find((item) => item.id === id)
  }

  function capabilityById(id: number | undefined): Capability | undefined {
    return data.value.capabilities.find((item) => item.id === id)
  }

  const modelById = computed(() => {
    const index = new Map<number, Model>()
    for (const model of data.value.models) {
      index.set(model.id, model)
    }
    return index
  })

  return {
    data,
    loaded,
    loading,
    error,
    load,
    refreshTags,
    purposeById,
    capabilityById,
    modelById,
  }
})
