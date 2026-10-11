import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import AppHeader from '@/components/AppHeader.vue'
import { i18n, setLocale } from '@/i18n'
import { usePreferencesStore } from '@/stores/preferences'

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(AppHeader, {
    global: {
      plugins: [pinia, i18n, ElementPlus],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
  return { wrapper, preferences: usePreferencesStore() }
}

beforeEach(() => {
  localStorage.clear()
  setLocale('zh')
  document.documentElement.classList.remove('dark')
})

describe('顶栏', () => {
  it('渲染平台名称与标语', () => {
    const { wrapper } = setup()
    expect(wrapper.text()).toContain('Prompt Benchmark')
    expect(wrapper.text()).toContain('提示词能力边界评测平台')
  })

  it('切换语言后文案随之改变', async () => {
    const { wrapper, preferences } = setup()
    preferences.setLocale('en')
    await nextTick()

    expect(wrapper.text()).toContain('Benchmarking the capability boundaries of language models')
    expect(wrapper.text()).toContain('Home')
    expect(wrapper.text()).not.toContain('提示词能力边界评测平台')
  })

  it('主题切换写入根元素类名与本地存储', async () => {
    const { wrapper } = setup()
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    await wrapper.get('button').trigger('click')

    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('pb.theme')).toBe('dark')
  })

  it('主题按钮的可访问名称随语言切换', () => {
    const { wrapper } = setup()
    expect(wrapper.get('button').attributes('aria-label')).toBe('主题')
  })
})
