export type AppArea = 'agent' | 'qq'

export const APP_AREAS = {
  agent: { name: 'Agent', path: '/chat' },
  qq: { name: 'QQ', path: '/qq' },
} as const

const AREA_KEY = 'caf.active-area'

export function readAppArea(): AppArea {
  try {
    return localStorage.getItem(AREA_KEY) === 'qq' ? 'qq' : 'agent'
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
