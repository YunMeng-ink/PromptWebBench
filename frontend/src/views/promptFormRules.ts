// 上传与编辑表单的字段规则。与视图分离，便于直接对规则本身测试。

import type { Purpose, Visibility } from '@/api/prompt'

/**
 * visiblePurposes 返回当前身份可选择的考察目的。
 *
 * 高风险考察目的只对开发者与管理员开放（后端 C-19 与需求 §8.7）。
 * 表单先行过滤，是为了让普通用户根本选不到，避免选中后被接口拒绝。
 */
export function visiblePurposes(purposes: Purpose[], isDeveloper: boolean): Purpose[] {
  return purposes.filter((item) => !item.is_high_risk || isDeveloper)
}

/**
 * normalizeVisibility 在身份不支持开发者可见时回退到公开。
 * 编辑既有提示词时后端返回的可见性原样带入，普通用户编辑自己早期留下的
 * 开发者可见内容时会被收敛到公开，避免提交必然被拒。
 */
export function normalizeVisibility(requested: Visibility, isDeveloper: boolean): Visibility {
  if (requested === 'developer' && !isDeveloper) {
    return 'public'
  }
  return requested
}
