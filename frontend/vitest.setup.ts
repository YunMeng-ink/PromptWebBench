import { vi } from 'vitest'

// jsdom 未实现 matchMedia，测试环境补一个最小实现。
// 生产代码不做该分支：目标浏览器（Chrome/Firefox/Safari/Edge 最近两个大版本）均提供该 API。
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  const stub = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }) as unknown as MediaQueryList
  window.matchMedia = stub as typeof window.matchMedia
}
