// 字典数据的进程内缓存。类别、能力维度、难度、标签与模型变动极少，
// 每个页面各拉一次会造成重复请求，这里统一缓存并允许手动刷新。

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  type Ability,
  type Category,
  type Difficulty,
  dictionaryApi,
  type Model,
  type Tag,
} from '@/api/prompt'

interface Dictionaries {
  categories: Category[]
  abilities: Ability[]
  difficulties: Difficulty[]
  tags: Tag[]
  models: Model[]
}

const empty: Dictionaries = {
  categories: [],
  abilities: [],
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
        const [categories, abilities, difficulties, tags, models] = await Promise.all([
          dictionaryApi.categories(),
          dictionaryApi.abilities(),
          dictionaryApi.difficulties(),
          dictionaryApi.tags(),
          dictionaryApi.models(),
        ])
        data.value = {
          categories: categories.items,
          abilities: abilities.items,
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

  function categoryById(id: number | undefined): Category | undefined {
    return data.value.categories.find((item) => item.id === id)
  }

  const modelById = computed(() => {
    const index = new Map<number, Model>()
    for (const model of data.value.models) {
      index.set(model.id, model)
    }
    return index
  })

  return { data, loaded, loading, error, load, categoryById, modelById }
})
