// 供应商预设：与 magpie 的供应商面板对齐，按上游地址识别厂商，
// 显示「名称 + 图标」而不是让用户盯着 base_url 看。
//
// hosts 支持 * 通配（如 bedrock-runtime.*.amazonaws.com）。匹配时先试更长、
// 更具体的模式，所以「套餐专用地址」会先于厂商主地址命中。

export type ProviderKind = 'vendor' | 'relay' | 'local'

export interface ProviderPreset {
  id: string
  name: string
  /** 列表里更短的名字；没有时用 name */
  short?: string
  icon: string
  kind: ProviderKind
  /** 名称下面的一行说明，例如「订阅」「Token Plan」 */
  note?: string
  /** 订阅/套餐类：和按量计费的 API Key 区分展示 */
  subscription?: boolean
  hosts: string[]
}

export const PROVIDER_PRESETS: ProviderPreset[] = [
  // ===== 厂商 =====
  { id: 'anthropic', name: 'Anthropic', icon: 'claude-color', kind: 'vendor', hosts: ['api.anthropic.com'] },
  { id: 'openai', name: 'OpenAI', icon: 'openai', kind: 'vendor', hosts: ['api.openai.com'] },
  { id: 'google', name: 'Google Gemini', icon: 'gemini-color', kind: 'vendor', note: 'Gemini API', hosts: ['generativelanguage.googleapis.com'] },
  { id: 'vertex', name: 'Vertex AI', icon: 'vertexai-color', kind: 'vendor', note: 'Google Cloud', hosts: ['*-aiplatform.googleapis.com', 'aiplatform.googleapis.com'] },
  { id: 'azure', name: 'Azure OpenAI', icon: 'azure-color', kind: 'vendor', hosts: ['*.openai.azure.com'] },
  { id: 'deepseek', name: 'DeepSeek', icon: 'deepseek-color', kind: 'vendor', hosts: ['api.deepseek.com'] },
  { id: 'xai', name: 'xAI', icon: 'xai', kind: 'vendor', hosts: ['api.x.ai'] },
  { id: 'moonshot', name: 'Kimi', icon: 'kimi', kind: 'vendor', hosts: ['api.moonshot.ai'] },
  { id: 'moonshot-cn', name: 'Kimi (China)', icon: 'kimi', kind: 'vendor', hosts: ['api.moonshot.cn'] },
  { id: 'kimi-code', name: 'Kimi Code', icon: 'kimi', kind: 'vendor', note: '订阅', subscription: true, hosts: ['api.kimi.ai'] },
  { id: 'kimi-code-cn', name: 'Kimi Code (China)', icon: 'kimi', kind: 'vendor', note: '订阅', subscription: true, hosts: ['api.kimi.com'] },
  { id: 'zhipu', name: 'Zhipu GLM', icon: 'zhipu-color', kind: 'vendor', hosts: ['open.bigmodel.cn'] },
  { id: 'zai', name: 'Z.ai', icon: 'zai', kind: 'vendor', hosts: ['api.z.ai'] },
  { id: 'minimax', name: 'MiniMax', icon: 'minimax-color', kind: 'vendor', hosts: ['api.minimax.io'] },
  { id: 'minimax-cn', name: 'MiniMax (China)', icon: 'minimax-color', kind: 'vendor', hosts: ['api.minimaxi.com', 'api.minimax.cn'] },
  { id: 'stepfun', name: 'StepFun', icon: 'stepfun-color', kind: 'vendor', hosts: ['api.stepfun.ai'] },
  { id: 'stepfun-cn', name: 'StepFun (China)', icon: 'stepfun-color', kind: 'vendor', hosts: ['api.stepfun.com'] },
  { id: 'xiaomi', name: 'Xiaomi MiMo', icon: 'mimocode', kind: 'vendor', note: 'Token Plan', hosts: ['*.xiaomimimo.com'] },
  { id: 'longcat', name: 'LongCat', icon: 'longcat', kind: 'vendor', hosts: ['api.longcat.chat'] },
  { id: 'baidu-qianfan', name: 'Baidu Qianfan', icon: 'baiducloud-color', kind: 'vendor', note: 'Token Plan', hosts: ['qianfan.baidubce.com'] },
  { id: 'tencent-cloud', name: 'Tencent Cloud', icon: 'tencentcloud-color', kind: 'vendor', note: 'Token Plan / 混元', hosts: ['*.tencentmaas.com'] },
  { id: 'huaweicloud', name: 'Huawei Cloud', icon: 'huaweicloud-color', kind: 'vendor', note: 'Token Plan', hosts: ['api.modelarts-maas.com'] },
  { id: 'volcengine', name: 'Volcengine Ark', icon: 'volcengine-color', kind: 'vendor', note: 'Coding / Agent Plan', hosts: ['ark.cn-beijing.volces.com', '*.volces.com'] },
  { id: 'qwen', name: 'Qwen', icon: 'qwen-color', kind: 'vendor', note: 'DashScope · 国际', hosts: ['dashscope-intl.aliyuncs.com'] },
  { id: 'qwen-cn', name: 'Qwen (China)', icon: 'qwen-color', kind: 'vendor', note: 'DashScope · 国内', hosts: ['dashscope.aliyuncs.com'] },
  { id: 'qwen-token-plan', name: 'Qwen Token Plan', short: 'Qwen Plan', icon: 'qwen-color', kind: 'vendor', note: '订阅', subscription: true, hosts: ['token-plan.maas.qianwenaiapi.com'] },
  { id: 'bailian-token-plan', name: 'Bailian Token Plan', short: 'Bailian Plan', icon: 'bailian-color', kind: 'vendor', note: '订阅', subscription: true, hosts: ['token-plan.cn-beijing.maas.aliyuncs.com'] },
  { id: 'qwencloud-token-plan', name: 'Qwen Cloud Token Plan', short: 'Qwen Cloud Plan', icon: 'qwen-color', kind: 'vendor', note: '订阅', subscription: true, hosts: ['token-plan.maas.qwencloudapi.com'] },
  { id: 'qwencloud', name: 'Qwen Cloud', icon: 'qwen-color', kind: 'vendor', note: '按量', hosts: ['*.maas.qwencloudapi.com'] },
  { id: 'mistral', name: 'Mistral', icon: 'mistral-color', kind: 'vendor', hosts: ['api.mistral.ai'] },
  { id: 'voyage', name: 'Voyage AI', icon: 'voyage-color', kind: 'vendor', note: '向量 / 重排', hosts: ['api.voyageai.com'] },
  { id: 'groq', name: 'Groq', icon: 'groq', kind: 'vendor', hosts: ['api.groq.com'] },
  { id: 'bedrock', name: 'Amazon Bedrock', icon: 'bedrock-color', kind: 'vendor', hosts: ['bedrock-runtime.*.amazonaws.com'] },
  { id: 'bedrock-mantle', name: 'Bedrock Mantle', icon: 'bedrock-color', kind: 'vendor', hosts: ['bedrock-mantle.*.api.aws'] },
  { id: 'ollama-cloud', name: 'Ollama Cloud', icon: 'ollama', kind: 'vendor', hosts: ['ollama.com'] },
  { id: 'nous', name: 'Nous Portal', icon: 'nousresearch', kind: 'vendor', hosts: ['inference-api.nousresearch.com'] },
  { id: 'typesafe', name: 'TypeSafe Jev', icon: 'typesafe', kind: 'vendor', note: '决策模型', hosts: ['api.typesafe.ai', 'typesafe.ai'] },

  // ===== 中转 / 聚合 =====
  { id: 'openrouter', name: 'OpenRouter', icon: 'openrouter', kind: 'relay', hosts: ['openrouter.ai'] },
  { id: 'opencode', name: 'OpenCode', icon: 'opencode', kind: 'relay', hosts: ['opencode.ai'] },
  { id: 'clinepass', name: 'ClinePass', icon: 'cline', kind: 'relay', hosts: ['api.cline.bot'] },
  { id: 'kilo', name: 'Kilo Gateway', icon: 'kilo', kind: 'relay', hosts: ['api.kilo.ai'] },
  { id: 'commandcode', name: 'Command Code', icon: 'commandcode', kind: 'relay', hosts: ['api.commandcode.ai'] },
  { id: 'together', name: 'Together AI', icon: 'together-color', kind: 'relay', hosts: ['api.together.xyz', 'api.together.ai'] },
  { id: 'fireworks', name: 'Fireworks', icon: 'fireworks-color', kind: 'relay', hosts: ['api.fireworks.ai'] },
  { id: 'siliconflow', name: 'SiliconFlow', icon: 'siliconcloud-color', kind: 'relay', hosts: ['api.siliconflow.cn', 'api.siliconflow.com'] },
  { id: 'nvidia', name: 'NVIDIA NIM', icon: 'nvidia-color', kind: 'relay', hosts: ['integrate.api.nvidia.com'] },
  { id: 'modelscope', name: 'ModelScope', icon: 'modelscope-color', kind: 'relay', hosts: ['api-inference.modelscope.cn'] },
  { id: 'aihubmix', name: 'AiHubMix', icon: 'aihubmix-color', kind: 'relay', hosts: ['aihubmix.com'] },
  { id: 'pipellm', name: 'PipeLLM', icon: 'pipellm-color', kind: 'relay', hosts: ['api.pipellm.ai'] },
  { id: '302ai', name: '302.AI', icon: 'ai302-color', kind: 'relay', hosts: ['api.302.ai'] },
  { id: 'cherryin', name: 'CherryIN', icon: 'cherryin-color', kind: 'relay', hosts: ['open.cherryin.ai'] },
  { id: 'yylx', name: '云雾 API', icon: 'yylx', kind: 'relay', hosts: ['yylx.io', '*.yylx.io'] },
  { id: 'novita', name: 'Novita', icon: 'novita-color', kind: 'relay', hosts: ['api.novita.ai'] },
  { id: 'vercel', name: 'Vercel AI Gateway', icon: 'vercel', kind: 'relay', hosts: ['ai-gateway.vercel.sh'] },
  { id: 'cloudflare', name: 'Cloudflare Workers AI', icon: 'cloudflare-color', kind: 'relay', hosts: ['api.cloudflare.com'] },
]

/** 内置引擎的图标与说明；custom 走上面的预设表。 */
export const ENGINE_PROVIDERS: Record<string, { icon: string; note?: string; subscription?: boolean }> = {
  geminiweb: { icon: 'gemini-color', note: '网页账号' },
  aistudio: { icon: 'google-color', note: 'Google 账号' },
  codex: { icon: 'codex-color', note: 'ChatGPT 订阅', subscription: true },
  antigravity: { icon: 'antigravity-color', note: 'Google 订阅', subscription: true },
  kiro: { icon: 'kiro-color', note: 'AWS 订阅', subscription: true },
  workbuddy: { icon: 'workbuddy-color', note: '订阅账号', subscription: true },
  teleagent: { icon: '', note: '天翼订阅', subscription: true },
}

export interface ResolvedProvider {
  name: string
  icon: string
  note?: string
  subscription?: boolean
  /** 识别出的预设；未知地址为 null */
  preset: ProviderPreset | null
  /** 原始地址主机名，未知时用作名称 */
  host?: string
}

/** 从任意 base URL / custom|<base_url> 里取主机名。 */
export function baseURLHost(value?: string | null): string {
  if (!value) return ''
  let raw = value.trim()
  if (raw.startsWith('custom|')) raw = raw.slice('custom|'.length)
  if (!/^https?:\/\//i.test(raw)) raw = `https://${raw}`
  try {
    return new URL(raw).hostname.toLowerCase()
  } catch {
    return raw.replace(/^https?:\/\//i, '').replace(/^www\./, '').split('/')[0].toLowerCase()
  }
}

const globToRegExp = (pattern: string) =>
  new RegExp(`^${pattern.split('*').map((s) => s.replace(/[.+?^${}()|[\]\\]/g, '\\$&')).join('.*')}$`)
const HOST_MATCHERS = PROVIDER_PRESETS.flatMap((preset) =>
  preset.hosts.map((host) => ({ preset, host, re: globToRegExp(host) })),
).sort((a, b) => b.host.length - a.host.length)

export function presetForHost(host: string): ProviderPreset | null {
  if (!host) return null
  for (const { preset, host: pattern, re } of HOST_MATCHERS) {
    if (re.test(host)) return preset
    // 没有通配的域名也接受子域名：api.deepseek.com 匹配 foo.api.deepseek.com
    if (!pattern.includes('*') && (host === pattern || host.endsWith(`.${pattern}`))) return preset
  }
  return null
}

/**
 * 解析一个账号/路由组的供应商展示。
 * - custom：按 base_url 命中预设，显示厂商名 + 图标；未命中则退回主机名。
 * - 内置引擎：按引擎表取图标和说明，名称仍用引擎名。
 */
export function resolveProvider(engine: string, providerOrURL?: string | null, fallbackName?: string): ResolvedProvider {
  if (engine === 'custom' || engine.startsWith('custom|')) {
    const raw = providerOrURL || (engine.startsWith('custom|') ? engine.slice('custom|'.length) : '')
    const host = baseURLHost(raw)
    const preset = presetForHost(host)
    if (preset) {
      return { name: preset.short || preset.name, icon: preset.icon, note: preset.note, subscription: preset.subscription, preset, host }
    }
    return { name: host || fallbackName || '第三方 Key', icon: '', preset: null, host }
  }
  const meta = ENGINE_PROVIDERS[engine]
  return {
    name: fallbackName || engine,
    icon: meta?.icon || '',
    note: meta?.note,
    subscription: meta?.subscription,
    preset: null,
  }
}
