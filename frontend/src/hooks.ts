import { useCallback, useEffect, useRef, useState } from 'react'
import { on } from './api'

/** 订阅后端 data:changed 事件，返回递增的版本号，组件据此重新拉取数据。 */
export function useDataVersion(): number {
  const [v, setV] = useState(0)
  useEffect(() => on('data:changed', () => setV((x) => x + 1)), [])
  return v
}

/** 简易提示条（带撤销）。 */
export function useSnack() {
  const [snack, setSnack] = useState<{ text: string; undo?: () => void } | null>(null)
  const timer = useRef<number>()
  const show = useCallback((text: string, undo?: () => void, ms = 5000) => {
    window.clearTimeout(timer.current)
    setSnack({ text, undo })
    timer.current = window.setTimeout(() => setSnack(null), ms)
  }, [])
  const hide = useCallback(() => { window.clearTimeout(timer.current); setSnack(null) }, [])
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return { snack, show, hide }
}

export function useDebounced<T>(value: T, ms: number): T {
  const [d, setD] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => setD(value), ms)
    return () => window.clearTimeout(id)
  }, [value, ms])
  return d
}
