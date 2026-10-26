import { useEffect, useMemo, useState } from 'react'
import { ExternalLink, PlugZap } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { CopyButton } from '@/components/copy'
import { MiniSelect } from '@/components/fields'
import { cn } from '@/lib/utils'
import type { ApiKey } from '@/lib/types'

/** 网关 /v1/codex/catalog 的条目（Codex 模型目录格式，snake_case）。 */
interface CatalogEntry {
  slug: string
  display_name?: string
  description?: string
  context_window?: number
  input_modalities?: string[]
  base_instructions?: string
  supports_parallel_tool_calls?: boolean
  supported_reasoning_levels?: { effort: string; description?: string }[]
}

/** CC Switch 供应商里保存的模型目录条目（camelCase，见 cc-switch 的 settings_config.modelCatalog）。 */
interface CcSwitchCatalogModel {
  model: string
  displayName: string
  contextWindow: number
  supportsParallelToolCalls: boolean
  inputModalities: string[]
  baseInstructions: string
  reasoningLevels: string[]
}

/** 把网关目录条目映射成 CC Switch 的 modelCatalog 结构。 */
export function toCatalogModel(entry: CatalogEntry): CcSwitchCatalogModel {
  const name = entry.display_name || entry.slug
  return {
    model: entry.slug,
    displayName: name,
    contextWindow: entry.context_window || 1000000,
    supportsParallelToolCalls: entry.supports_parallel_tool_calls ?? true,
    inputModalities: entry.input_modalities?.length ? entry.input_modalities : ['text'],
    baseInstructions:
      entry.base_instructions ||
      `You are Codex, a coding agent based on ${name}. You and the user share the same workspace and collaborate to achieve the user's goals.`,
    reasoningLevels: entry.supported_reasoning_levels?.length
      ? entry.supported_reasoning_levels.map((l) => l.effort)
      : ['none', 'medium', 'high'],
  }
}

/** 导入目标应用。 */
export type CcSwitchApp = 'codex' | 'claude'

/** UTF-8 安全的 base64（配置里含中文/非 ASCII 时 btoa 直接编码会报错）。 */
function utf8Base64(s: string): string {
  return btoa(String.fromCharCode(...new TextEncoder().encode(s)))
}

/** 生成 Codex 的 config.toml：wire_api=responses（Codex 已移除 chat），model_catalog_url 动态拉模型。 */
export function buildCcSwitchTOML(baseURL: string, apiKey: string, model: string): string {
  return [
    // 必须显式选路到 custom：缺这一行 Codex 用内置 openai 提供方，会把网关 Key 发到 api.openai.com。
    // CC Switch 导入时也会补这一行（deeplink/codex_link.rs render），手动粘贴就全靠它。
    'model_provider = "custom"',
    `model = "${model}"`,
    'model_reasoning_effort = "medium"',
    '[model_providers.custom]',
    'name = "web2api"',
    `base_url = "${baseURL}/v1"`,
    'wire_api = "responses"',
    // 直连 /v1/responses 时 Codex 需带 Bearer；false 时不会发 Key，网关会 401。
    'requires_openai_auth = true',
    `model_catalog_url = "${baseURL}/v1/codex/catalog?key=${apiKey}"`,
    '',
  ].join('\n')
}

/**
 * 生成 ccswitch:// Deep Link（V1 协议），CC Switch v4+ 支持一键导入。
 * - codex：内嵌 {"auth","config","modelCatalog"} 配置（wire_api=responses +
 *   model_catalog_url；apiFormat=openai_responses 直连网关 /v1/responses，无需本地路由）
 * - claude：Claude Code 原生说 Anthropic Messages，web2api 只输出 OpenAI Chat，
 *   apiFormat=openai_chat 让 CC Switch 自动开启本地路由做协议转换
 *
 * 协议本身只有单数 model 参数，没有模型数组字段；模型列表只能通过 config
 * 载荷里的 modelCatalog 带过去，否则 CC Switch 的模型列表是空的。
 */
export function buildCcSwitchLink(
  baseURL: string,
  apiKey: string,
  model: string,
  app: CcSwitchApp = 'codex',
  name = 'web2api',
  catalog: CcSwitchCatalogModel[] = [],
): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app,
    name,
    endpoint: `${baseURL}/v1`,
    apiKey,
    model,
    // Codex 只认 Responses；Claude Code 仍需本地路由把 Anthropic Messages 翻成 Chat。
    apiFormat: app === 'codex' ? 'openai_responses' : 'openai_chat',
  })
  if (app === 'codex') {
    const payload: Record<string, unknown> = {
      auth: { OPENAI_API_KEY: apiKey },
      config: buildCcSwitchTOML(baseURL, apiKey, model),
    }
    if (catalog.length) payload.modelCatalog = { models: catalog }
    params.set('configFormat', 'json')
    params.set('config', utf8Base64(JSON.stringify(payload)))
  }
  return `ccswitch://v1/import?${params.toString()}`
}

/**
 * 「导入 CC Switch」弹窗：为单个密钥生成 Codex / Claude Code 供应商导入配置。
 * 模型列表实时取 /v1/codex/catalog（该 Key 的白名单与上下架过滤在网关侧生效），
 * 同时写进 Deep Link 的 modelCatalog，导入后 CC Switch 的模型列表就是这些模型。
 * Codex 另可用 model_catalog_url 动态刷新；Claude Code 经 CC Switch 本地路由
 * 把 Anthropic Messages 转成 Chat Completions。
 */
export function CcSwitchImportDialog({ keyItem, onOpenChange }: { keyItem: ApiKey | null; onOpenChange: (o: boolean) => void }) {
  const [app, setApp] = useState<CcSwitchApp>('codex')
  const [entries, setEntries] = useState<CatalogEntry[]>([])
  const [model, setModel] = useState('')
  const [err, setErr] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    if (!keyItem) return
    setApp('codex')
    setEntries([])
    setModel('')
    setErr('')
    setLoaded(false)
    fetch('/v1/codex/catalog', { headers: { Authorization: `Bearer ${keyItem.key}` } })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((d) => {
        const list = ((d.models || []) as CatalogEntry[]).filter((m) => m.slug)
        setEntries(list)
        if (list.length) setModel(list[0].slug)
      })
      .catch((e) => setErr(String((e as Error).message || e)))
      .finally(() => setLoaded(true))
  }, [keyItem])

  const models = useMemo(() => entries.map((m) => ({ id: m.slug, name: m.display_name || m.slug })), [entries])
  const catalog = useMemo(() => entries.map(toCatalogModel), [entries])

  // 切到 Claude Code 时优先选号池里的 claude-* 模型（Claude Code 对模型名有惯例）
  const switchApp = (next: CcSwitchApp) => {
    setApp(next)
    if (next === 'claude' && models.length && !model.startsWith('claude-')) {
      const claudeModel = models.find((m) => m.id.startsWith('claude-'))
      if (claudeModel) setModel(claudeModel.id)
    }
  }

  const baseURL = typeof window !== 'undefined' ? window.location.origin : ''
  const toml = useMemo(() => (keyItem && model ? buildCcSwitchTOML(baseURL, keyItem.key, model) : ''), [keyItem, model, baseURL])
  const link = useMemo(
    () => (keyItem && model ? buildCcSwitchLink(baseURL, keyItem.key, model, app, 'web2api', catalog) : ''),
    [keyItem, model, baseURL, app, catalog],
  )

  return (
    <Dialog open={!!keyItem} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-[15px]">
            <PlugZap className="size-4" /> 导入 CC Switch
          </DialogTitle>
          <DialogDescription className="text-[13px]">
            把这个密钥一键导入 CC Switch。Codex 直连网关的 Responses 协议（wire_api=responses）；Claude Code 经本地路由把 Anthropic Messages 转成 Chat Completions。该密钥可用的模型会随导入一起写入 CC Switch 的模型列表。
          </DialogDescription>
        </DialogHeader>
        {keyItem && (
          <div className="min-w-0 space-y-4">
            {!loaded ? (
              <div className="py-6 text-center text-xs text-muted-foreground">正在拉取该密钥可用的模型…</div>
            ) : err ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">拉取模型列表失败：{err}。请确认密钥有效后重试。</div>
            ) : !models.length ? (
              <div className="rounded-md border border-warning/40 bg-warning/10 p-3 text-xs">该密钥当前没有可用的对话模型（可能被白名单限制或模型已下架），导入后也无法使用。</div>
            ) : (
              <>
                <div className="inline-flex rounded-md border p-0.5">
                  {([['codex', 'Codex'], ['claude', 'Claude Code']] as [CcSwitchApp, string][]).map(([id, label]) => (
                    <button
                      key={id}
                      type="button"
                      onClick={() => switchApp(id)}
                      className={cn(
                        'rounded-[5px] px-3 py-1 text-xs font-medium transition-colors',
                        app === id ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:text-foreground',
                      )}
                    >
                      {label}
                    </button>
                  ))}
                </div>
                <div className="flex items-center gap-2">
                  <span className="shrink-0 text-xs text-muted-foreground">{app === 'claude' ? '模型' : '默认模型'}</span>
                  <MiniSelect
                    value={model}
                    onChange={setModel}
                    className="flex-1"
                    options={models.map((m) => ({ value: m.id, label: m.name }))}
                  />
                  <span className="shrink-0 text-xs text-muted-foreground">{models.length} 个对话模型</span>
                </div>
                <Button className="w-full" onClick={() => { window.location.href = link }}>
                  <ExternalLink /> 一键导入 CC Switch（{app === 'claude' ? 'Claude Code' : 'Codex'}）
                </Button>
                <p className="text-[11px] leading-relaxed text-muted-foreground">
                  {app === 'codex'
                    ? <>点击后浏览器会请求打开 CC Switch，确认导入即完成。配置使用 wire_api=responses 直连网关 /v1/responses；Codex 经 model_catalog_url 动态拉取模型目录，号池变化无需重新导入。需 CC Switch v4+。</>
                    : <>Claude Code 原生使用 Anthropic Messages 协议，CC Switch 导入时会开启本地路由，自动把它转换成 web2api 的 Chat Completions 接口。需 CC Switch v4+。</>}
                </p>
                <details className="group min-w-0">
                  <summary className="cursor-pointer text-xs text-muted-foreground select-none group-open:mb-2">手动配置（Deep Link 打不开时）</summary>
                  <div className="min-w-0 space-y-2">
                    {app === 'codex' && (
                      <div className="relative rounded-md border bg-[#0a0a0c]">
                        <div className="flex items-center justify-between border-b px-3 py-1.5">
                          <span className="text-[11px] text-muted-foreground">config.toml · 在 CC Switch 的 Codex 供应商编辑器中粘贴</span>
                          <CopyButton value={toml} label="配置已复制" />
                        </div>
                        <pre className="max-h-44 overflow-auto p-3 font-mono text-[11px] leading-relaxed break-all whitespace-pre-wrap text-[#c9cbe0]">{toml}</pre>
                      </div>
                    )}
                    {app === 'claude' && (
                      <div className="rounded-md border bg-[#0a0a0c] p-3 text-[11px] leading-relaxed text-[#c9cbe0]">
                        在 CC Switch 的 Claude 供应商中新建：Base URL 填 <code className="font-mono">{baseURL}/v1</code>，API Key 填该密钥，上游格式选 <b>Chat Completions</b> 并开启路由。注意：Claude Code 不能把 ANTHROPIC_BASE_URL 直接指向 web2api——网关没有 /v1/messages 端点，必须经 CC Switch 路由转换。
                      </div>
                    )}
                    <div className="flex items-center gap-1 rounded-md border bg-[#0a0a0c] px-3 py-1.5">
                      <code className="min-w-0 flex-1 truncate font-mono text-[10.5px] text-muted-foreground">{link}</code>
                      <CopyButton value={link} label="链接已复制" />
                    </div>
                    <p className="text-[11px] text-muted-foreground">链接与配置中包含完整密钥，请勿公开分享。</p>
                  </div>
                </details>
              </>
            )}
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
