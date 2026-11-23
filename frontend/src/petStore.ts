/** 狗狗只在本机浏览器里记的小状态（每天的问候 / 纪念日 / 喝水提醒时间）。存储不可用时退化为不记（最多重复说一次）。 */
export const petStore = {
  get: (k: string) => { try { return localStorage.getItem('deskpin.pet.' + k) } catch { return null } },
  set: (k: string, v: string) => { try { localStorage.setItem('deskpin.pet.' + k, v) } catch { /* 忽略 */ } },
}
