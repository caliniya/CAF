export type AppArea = 'agent' | 'ext'

export const APP_AREAS = {
  agent: { name: 'Agent', path: '/chat' },
  ext: { name: 'Ext', path: '/ext' },
} as const

const AREA_KEY = 'caf.active-area'

export function readAppArea(): AppArea {
  try {
    const saved = localStorage.getItem(AREA_KEY)
    return saved === 'ext' || saved === 'qq' ? 'ext' : 'agent'
  } catch {
    return 'agent'
  }
}

export function saveAppArea(area: AppArea) {
  try {
    localStorage.setItem(AREA_KEY, area)
  } catch {
    // 无法保存时仍可在本次使用中切换界面。
  }
}
