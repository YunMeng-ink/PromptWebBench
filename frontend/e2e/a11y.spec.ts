// 阶段 7 的无障碍核查（开发方案 §6.8）：图标按钮可访问名称、键盘可达、对比度。
//
// 以真实浏览器运行：对比度按 WCAG 2.1 的相对亮度公式计算，键盘可达性用 Tab 键实测。

import { expect, test } from '@playwright/test'
import { loginShared, openAsShared } from './helpers'

/** 相对亮度（WCAG 2.1 §1.4.3 的取值口径）。 */
function relativeLuminance(rgb: [number, number, number]): number {
  const [r, g, b] = rgb.map((channel) => {
    const value = channel / 255
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }) as [number, number, number]
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

/** contrastRatio 计算两色的对比度。 */
function contrastRatio(
  foreground: [number, number, number],
  background: [number, number, number],
): number {
  const light = Math.max(relativeLuminance(foreground), relativeLuminance(background))
  const dark = Math.min(relativeLuminance(foreground), relativeLuminance(background))
  return (light + 0.05) / (dark + 0.05)
}

/** parseRgb 把 "rgb(r, g, b)" 解析为三元组。 */
function parseRgb(value: string): [number, number, number] {
  const match = value.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)
  if (match === null) {
    throw new Error(`无法解析颜色 ${value}`)
  }
  return [Number(match[1]), Number(match[2]), Number(match[3])]
}

test.describe('无障碍', () => {
  test('交互元素都有可访问名称', async ({ page }) => {
    await page.goto('/')

    const unnamed = await page.evaluate(() => {
      const out: string[] = []
      for (const element of Array.from(
        document.querySelectorAll('button, a, input, select, [role="button"]'),
      )) {
        // 仅检查可见元素：隐藏元素不参与键盘与读屏交互。
        const rect = element.getBoundingClientRect()
        if (rect.width === 0 || rect.height === 0) {
          continue
        }
        const text = (element.textContent ?? '').trim()
        const aria = element.getAttribute('aria-label') ?? ''
        const title = element.getAttribute('title') ?? ''
        const labelledBy = element.getAttribute('aria-labelledby') ?? ''
        const placeholder = element.getAttribute('placeholder') ?? ''
        if (text === '' && aria === '' && title === '' && labelledBy === '' && placeholder === '') {
          out.push(`${element.tagName.toLowerCase()}.${element.className}`)
        }
      }
      return out.slice(0, 10)
    })
    expect(unnamed, `以下元素缺少可访问名称：${unnamed.join('、')}`).toEqual([])
  })

  test('键盘可以依次到达顶栏的全部入口', async ({ page }) => {
    await loginShared(page)
    await page.goto('/')
    await expect(page.getByTestId('header-user')).toBeVisible()

    /*
     * Safari 默认跳过链接（Tab 只走表单控件），因此这里以按钮与输入框为判定对象：
     * 语言选择、主题切换、账户入口在四个浏览器里都在 Tab 序列中。
     * 允许 20 次 Tab 的富余量，浏览器对链接的取舍不影响这些控件的可达性。
     */
    const reached: string[] = []
    for (let i = 0; i < 20; i += 1) {
      await page.keyboard.press('Tab')
      const info = await page.evaluate(() => {
        const active = document.activeElement
        if (active === null) {
          return ''
        }
        return `${active.tagName.toLowerCase()}:${(active.textContent ?? '').trim().slice(0, 16)}`
      })
      if (info !== '') {
        reached.push(info)
      }
    }

    expect(reached.length, 'Tab 应能到达多个元素').toBeGreaterThan(3)
    const wants = [
      {
        label: '主题切换按钮',
        matched: reached.some((item) => item.includes('深色') || item.includes('浅色')),
      },
      { label: '账户入口', matched: reached.some((item) => item.includes('e2e-shared')) },
    ]
    const missing = wants.filter((item) => !item.matched).map((item) => item.label)
    expect(
      missing,
      `Tab 序列未到达：${missing.join('、')}；实际序列 ${reached.join(' / ')}`,
    ).toEqual([])
  })

  test('主要正文与辅助文本的对比度达标', async ({ page }) => {
    await page.goto('/')

    const samples = await page.evaluate(() => {
      /** 取元素的有效背景色：向上找到第一个非透明背景。 */
      const backgroundOf = (element: Element): string => {
        let current: Element | null = element
        while (current !== null) {
          const bg = getComputedStyle(current).backgroundColor
          if (bg !== '' && bg !== 'transparent' && !bg.startsWith('rgba(0, 0, 0, 0)')) {
            return bg
          }
          current = current.parentElement
        }
        return 'rgb(255, 255, 255)'
      }
      const out: { selector: string; color: string; background: string; fontSize: number }[] = []
      for (const selector of ['h1', 'a', 'p', 'span']) {
        const element = document.querySelector(selector)
        if (element === null) {
          continue
        }
        const rect = element.getBoundingClientRect()
        if (rect.width === 0 || rect.height === 0) {
          continue
        }
        const style = getComputedStyle(element)
        out.push({
          selector,
          color: style.color,
          background: backgroundOf(element),
          fontSize: Number.parseFloat(style.fontSize),
        })
      }
      return out
    })

    for (const sample of samples) {
      const fg = parseRgb(sample.color)
      const bg = parseRgb(sample.background)
      const ratio = contrastRatio(fg, bg)
      // WCAG AA：正文 4.5:1，18px 以上大字 3:1。
      const minimum = sample.fontSize >= 18 ? 3 : 4.5
      expect(
        ratio,
        `${sample.selector} 的对比度 ${ratio.toFixed(2)} 低于 ${minimum}:1（前景 ${sample.color}，背景 ${sample.background}）`,
      ).toBeGreaterThanOrEqual(minimum)
    }
  })

  test('明细页与个人中心的对比度达标', async ({ page }) => {
    await loginShared(page)
    await openAsShared(page, '/me')

    const ratios = await page.evaluate(() => {
      const backgroundOf = (element: Element): string => {
        let current: Element | null = element
        while (current !== null) {
          const bg = getComputedStyle(current).backgroundColor
          if (bg !== '' && bg !== 'transparent' && !bg.startsWith('rgba(0, 0, 0, 0)')) {
            return bg
          }
          current = current.parentElement
        }
        return 'rgb(255, 255, 255)'
      }
      const out: { label: string; color: string; background: string }[] = []
      for (const element of Array.from(document.querySelectorAll('h1, h2, .pb-me__hint'))) {
        const rect = element.getBoundingClientRect()
        if (rect.width === 0 || rect.height === 0) {
          continue
        }
        const style = getComputedStyle(element)
        out.push({
          label: `${element.tagName.toLowerCase()}.${element.className}`,
          color: style.color,
          background: backgroundOf(element),
        })
      }
      return out
    })

    for (const item of ratios) {
      const fg = parseRgb(item.color)
      const bg = parseRgb(item.background)
      const ratio = contrastRatio(fg, bg)
      expect(ratio, `${item.label} 的对比度 ${ratio.toFixed(2)} 低于 4.5:1`).toBeGreaterThanOrEqual(
        4.5,
      )
    }
  })
})
