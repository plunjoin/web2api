import { CopyButton } from '@/components/copy'

/** 接入示例：OpenAI 兼容接口，直接复制即可调用。 */
export function QuickStart({ apiKey, model = 'gemini-3.5-flash-lite' }: { apiKey?: string; model?: string }) {
  const base = typeof window !== 'undefined' ? window.location.origin : ''
  const key = apiKey || 'sk-你的密钥'
  const snippet = `curl ${base}/v1/chat/completions \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${model}","messages":[{"role":"user","content":"你好"}]}'`
  return (
    <div className="relative rounded-md border bg-[#0a0a0c]">
      <div className="flex items-center justify-between border-b px-3 py-1.5">
        <span className="text-[11px] text-muted-foreground">OpenAI 兼容 · Base URL {base}/v1</span>
        <CopyButton value={snippet} label="示例已复制" />
      </div>
      <pre className="overflow-x-auto p-3 font-mono text-[11.5px] leading-relaxed text-[#c9cbe0]">{snippet}</pre>
    </div>
  )
}
