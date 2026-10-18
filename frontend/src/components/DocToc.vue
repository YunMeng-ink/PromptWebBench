<script setup lang="ts">
// 文档目录：右下角的可折叠浮层，点击条目平滑滚动到对应章节。
// 章节位置按顶栏的实际高度计算，窄屏下的两行顶栏也不会遮住标题。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

interface DocTocSection {
  id: string
  title: string
  index?: string
}

const props = defineProps<{
  sections: readonly DocTocSection[]
  label: string
  closeLabel: string
}>()

const open = ref(false)
const activeID = ref('')
let frame = 0

/** headerOffset 返回顶栏高度与标题上方留白之和。 */
function headerOffset(): number {
  const header = document.querySelector<HTMLElement>('.pb-header')
  return (header?.getBoundingClientRect().height ?? 68) + 28
}

/** updateActive 找出当前滚动位置所在章节。 */
function updateActive(): void {
  const threshold = headerOffset() + 8
  let current = props.sections[0]?.id ?? ''
  for (const section of props.sections) {
    const element = document.getElementById(section.id)
    if (element === null) {
      continue
    }
    if (element.getBoundingClientRect().top <= threshold) {
      current = section.id
    } else {
      break
    }
  }
  activeID.value = current
}

function onScroll(): void {
  if (frame !== 0) {
    return
  }
  frame = window.requestAnimationFrame(() => {
    frame = 0
    updateActive()
  })
}

function onResize(): void {
  updateActive()
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    open.value = false
  }
}

/** goTo 将目标章节滚到顶栏下方，并同步地址栏片段。 */
function goTo(id: string): void {
  const target = document.getElementById(id)
  if (target === null) {
    return
  }
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const top = target.getBoundingClientRect().top + window.scrollY - headerOffset()
  window.scrollTo({ top: Math.max(0, top), behavior: reduceMotion ? 'auto' : 'smooth' })
  activeID.value = id
  window.history.replaceState(null, '', `#${id}`)
}

const currentIndex = computed(() => {
  const position = props.sections.findIndex((section) => section.id === activeID.value)
  return position === -1 ? '' : (props.sections[position]?.index ?? '')
})

onMounted(() => {
  updateActive()
  window.addEventListener('scroll', onScroll, { passive: true })
  window.addEventListener('resize', onResize)
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  window.removeEventListener('resize', onResize)
  window.removeEventListener('keydown', onKeydown)
  if (frame !== 0) {
    window.cancelAnimationFrame(frame)
  }
})
</script>

<template>
  <div class="pb-toc" :class="{ 'pb-toc--open': open }">
    <Transition name="pb-toc-panel">
      <nav
        v-if="open"
        id="pb-doc-toc"
        class="pb-toc__panel"
        :aria-label="label"
        @keydown.esc="open = false"
      >
        <header class="pb-toc__head">
          <span>{{ label }}</span>
          <button
            type="button"
            class="pb-toc__close"
            :aria-label="closeLabel"
            :title="closeLabel"
            @click="open = false"
          >
            ×
          </button>
        </header>
        <ol class="pb-toc__list">
          <li v-for="(section, position) in sections" :key="section.id">
            <a
              :href="`#${section.id}`"
              :class="{ 'is-active': activeID === section.id }"
              :aria-current="activeID === section.id ? 'location' : undefined"
              @click.prevent="goTo(section.id)"
            >
              <span class="pb-toc__index">
                {{ section.index ?? String(position + 1).padStart(2, '0') }}
              </span>
              <span class="pb-toc__title">{{ section.title }}</span>
            </a>
          </li>
        </ol>
      </nav>
    </Transition>

    <button
      type="button"
      class="pb-toc__toggle"
      :aria-expanded="open"
      aria-controls="pb-doc-toc"
      :aria-label="open ? closeLabel : label"
      @click="open = !open"
    >
      <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" aria-hidden="true">
        <path d="M4 6h16M4 12h16M4 18h10" />
      </svg>
      <span class="pb-toc__toggle-label">{{ label }}</span>
      <span v-if="currentIndex !== ''" class="pb-toc__current">{{ currentIndex }}</span>
    </button>
  </div>
</template>

<style scoped>
.pb-toc {
  align-items: flex-end;
  bottom: var(--pb-space-lg);
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  position: fixed;
  right: var(--pb-space-lg);
  z-index: 25;
}

.pb-toc__toggle {
  align-items: center;
  background: var(--pb-glass);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  box-shadow: var(--pb-shadow-md);
  color: var(--pb-ink);
  cursor: pointer;
  display: inline-flex;
  font-family: inherit;
  font-size: 13px;
  gap: 8px;
  padding: 9px 14px;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease),
    transform var(--pb-duration) var(--pb-ease);
  backdrop-filter: blur(14px) saturate(1.3);
  -webkit-backdrop-filter: blur(14px) saturate(1.3);
}

.pb-toc__toggle:hover {
  background: var(--pb-surface);
  border-color: var(--pb-line-strong);
  transform: translateY(-1px);
}

.pb-toc__toggle-label {
  letter-spacing: 0.02em;
}

.pb-toc__current {
  background: var(--pb-accent-soft);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-accent-strong);
  font-family: var(--pb-font-mono);
  font-size: 10px;
  padding: 1px 7px;
}

.pb-toc__panel {
  background: var(--pb-glass);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-lg);
  display: flex;
  flex-direction: column;
  max-height: min(68vh, 560px);
  overflow: hidden;
  width: min(288px, calc(100vw - 32px));
  backdrop-filter: blur(18px) saturate(1.35);
  -webkit-backdrop-filter: blur(18px) saturate(1.35);
}

.pb-toc__head {
  align-items: center;
  border-bottom: 1px solid var(--pb-line);
  color: var(--pb-ink-faint);
  display: flex;
  font-size: 10px;
  font-weight: 600;
  justify-content: space-between;
  letter-spacing: 0.16em;
  padding: 10px 10px 10px 14px;
  text-transform: uppercase;
}

.pb-toc__close {
  align-items: center;
  background: transparent;
  border: 0;
  border-radius: var(--pb-radius-sm);
  color: var(--pb-ink-faint);
  cursor: pointer;
  display: inline-flex;
  font-size: 18px;
  height: 24px;
  justify-content: center;
  line-height: 1;
  width: 24px;
}

.pb-toc__close:hover {
  background: var(--pb-surface-muted);
  color: var(--pb-ink);
}

.pb-toc__list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  list-style: none;
  margin: 0;
  overflow-y: auto;
  padding: var(--pb-space-xs);
}

.pb-toc__list a {
  align-items: baseline;
  border-left: 2px solid transparent;
  border-radius: var(--pb-radius-sm);
  color: var(--pb-ink-soft);
  display: flex;
  font-size: 12.5px;
  gap: 9px;
  line-height: 1.5;
  padding: 6px 8px;
  transition:
    background-color var(--pb-duration-fast) var(--pb-ease),
    border-color var(--pb-duration-fast) var(--pb-ease),
    color var(--pb-duration-fast) var(--pb-ease);
}

.pb-toc__list a:hover {
  background: var(--pb-surface-muted);
  color: var(--pb-ink);
}

.pb-toc__list a.is-active {
  background: var(--pb-accent-soft);
  border-left-color: var(--pb-accent);
  color: var(--pb-accent-strong);
}

.pb-toc__index {
  color: var(--pb-ink-faint);
  flex: none;
  font-family: var(--pb-font-mono);
  font-size: 9.5px;
  letter-spacing: 0.04em;
}

.pb-toc__title {
  min-width: 0;
}

.pb-toc-panel-enter-active,
.pb-toc-panel-leave-active {
  transition:
    opacity 180ms var(--pb-ease-out),
    transform 180ms var(--pb-ease-out);
  transform-origin: bottom right;
}

.pb-toc-panel-enter-from,
.pb-toc-panel-leave-to {
  opacity: 0;
  transform: translateY(6px) scale(0.98);
}

@media (max-width: 640px) {
  .pb-toc {
    bottom: var(--pb-space-md);
    right: var(--pb-space-md);
  }

  .pb-toc__toggle-label {
    display: none;
  }
}
</style>
