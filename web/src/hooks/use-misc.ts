import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'

export function useDebounced<T>(value: T, delay = 300) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setV(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return v
}

/** ?new=1 打开创建弹窗（命令面板的“新建”操作使用），关闭时清掉参数。 */
export function useNewParam(): [boolean, (open: boolean) => void] {
  const [params, setParams] = useSearchParams()
  const open = params.get('new') === '1'
  const set = (v: boolean) => {
    const next = new URLSearchParams(params)
    if (v) next.set('new', '1')
    else next.delete('new')
    setParams(next, { replace: true })
  }
  return [open, set]
}
