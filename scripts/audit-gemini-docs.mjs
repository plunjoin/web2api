// Rebuild the field inventory from the saved official OpenAPI, without network access.
import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const snapshot = 'docs/public/reference/gemini-interactions.openapi.json'
const raw = fs.readFileSync(path.join(root, snapshot))
const spec = JSON.parse(raw)
const schemas = spec.components.schemas
const sha256 = crypto.createHash('sha256').update(raw).digest('hex')
const provenance = JSON.parse(fs.readFileSync(path.join(root, 'docs/public/reference/gemini-sources.json'), 'utf8'))
for (const source of provenance.sources) {
  const bytes = fs.readFileSync(path.join(root, 'docs/public/reference', source.file))
  if (crypto.createHash('sha256').update(bytes).digest('hex') !== source.sha256) {
    throw new Error(`Source checksum differs from provenance: ${source.file}. Update the source record before regenerating.`)
  }
}
const endpoints = Object.entries(spec.paths)
const methods = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options', 'trace']
const used = new Set()
const input = new Set()
const output = new Set()

function visit(value, found) {
  if (!value || typeof value !== 'object') return
  if (value.$ref) {
    const prefix = '#/components/schemas/'
    if (value.$ref.startsWith(prefix)) {
      const name = value.$ref.slice(prefix.length)
      if (!schemas[name]) throw new Error(`Unresolved schema: ${name}`)
      if (!found.has(name)) {
        found.add(name)
        visit(schemas[name], found)
      }
    }
  }
  for (const [key, child] of Object.entries(value)) {
    if (!['example', 'examples', 'x-codeSamples'].includes(key)) visit(child, found)
  }
}
for (const [, item] of endpoints) {
  visit(item, used)
  for (const method of methods) {
    const operation = item[method]
    if (!operation) continue
    visit(operation.requestBody, input)
    visit(operation.responses, output)
  }
}

// Include every component, even schemas retained by Google for future operations.
for (const name of Object.keys(schemas)) {
  used.add(name)
  visit(schemas[name], used)
}

const esc = value => String(value ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;')
  .replaceAll('>', '&gt;').replaceAll('|', '&#124;').replaceAll('{', '&#123;')
  .replaceAll('}', '&#125;').replace(/\r?\n/g, ' ').trim()
const code = value => `<code>${esc(value).replaceAll('`', '&#96;')}</code>`
const anchor = name => `schema-${name.toLowerCase()}`
function constraints(value) {
  return ['default', 'minimum', 'maximum', 'exclusiveMinimum', 'exclusiveMaximum', 'minLength', 'maxLength', 'pattern', 'minItems', 'maxItems', 'uniqueItems', 'nullable']
    .filter(k => Object.hasOwn(value, k)).map(k => `${k}=${JSON.stringify(value[k])}`)
}
function type(value) {
  if (value.$ref) {
    const name = value.$ref.split('/').at(-1)
    return `[${name}](#${anchor(name)})`
  }
  if (Object.hasOwn(value, 'const')) return code(JSON.stringify(value.const))
  const alternatives = value.oneOf ?? value.anyOf ?? value.allOf
  if (alternatives) return alternatives.map(type).join(value.allOf ? ' &amp; ' : ' / ')
  if (value.enum) return value.enum.map(v => code(JSON.stringify(v))).join(' / ')
  if (value.type === 'array') return `Array&lt;${type(value.items ?? {})}&gt;`
  if (value.type === 'object' && value.additionalProperties) {
    return `Map&lt;string, ${type(value.additionalProperties === true ? {} : value.additionalProperties)}&gt;`
  }
  return code((value.type ?? 'any') + (value.format ? ` (${value.format})` : ''))
}

// Include inline objects and objects nested inside unions, arrays and map values.
function rows(value, prefix = '', required = [], result = []) {
  for (const [name, property] of Object.entries(value.properties ?? {})) {
    const location = prefix ? `${prefix}.${name}` : name
    result.push({ location, property, required: (value.required ?? required).includes(name) })
    rows(property, location, [], result)
  }
  for (const keyword of ['oneOf', 'anyOf', 'allOf']) {
    for (const [index, branch] of (value[keyword] ?? []).entries()) {
      rows(branch, `${prefix || '$'} (${keyword}[${index}])`, [], result)
    }
  }
  if (value.items) rows(value.items, `${prefix}[]`, [], result)
  if (value.additionalProperties && typeof value.additionalProperties === 'object') {
    rows(value.additionalProperties, `${prefix}.*`, [], result)
  }
  return result
}
function status(name, field) {
  if (['ModelInteraction', 'AgentInteraction'].includes(name)) {
    if (field === 'model') return '映射：聊天 model（实际模型目录决定）'
    if (field === 'input') return '部分：messages 仅文本；没有 Content/Step JSON 接口'
    if (field === 'system_instruction') return '映射：messages 中的 system 文本'
    if (field === 'stream') return '部分：聊天 SSE；不是 Interactions 事件流'
  }
  if (name === 'GenerationConfig' && field === 'max_output_tokens') return '映射：聊天 max_tokens'
  if (name === 'GenerationConfig' && ['temperature', 'top_p'].includes(field)) return '旧字段：同名聊天参数传入私有协议；新官方字段已废弃'
  if (name === 'TextContent' && field === 'text') return '部分：messages.content 文本 / 输出 content 文本'
  if (name === 'VideoResponseFormat' && ['aspect_ratio', 'resolution', 'duration'].includes(field)) {
    return '相似能力：Veo aspect_ratio / resolution / seconds；没有此对象映射'
  }
  if (input.has(name)) return '未开放：聊天 HTTP 不接收此官方结构；见差距说明'
  return '未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务'
}
const lines = [
  '# Gemini 官方字段完整对照', '',
  '> 此页由 `scripts/audit-gemini-docs.mjs` 从官方 OpenAPI 快照生成。请通过修改快照并重新生成来更新此页。', '',
  `对照日期：${provenance.retrieved_date}（北京时间）。范围为保存的官方 OpenAPI 中全部 ${endpoints.length} 个路径、${endpoints.reduce((sum, [, item]) => sum + methods.filter(method => item[method]).length, 0)} 个操作和全部组件：Interactions 创建 / 获取 / 取消 / 删除、agents、voices、environments、webhooks、triggers、credentials 等资源，以及请求、响应、错误和 SSE schema。快照未定义的独立 Files / models API 通过官方后端通用路由转发，使用流程见中文参考详解。`, '',
  '启用 gemini_api 后，官方路由完整转发本页全部字段、联合类型及未来扩展字段，由 Google 校验并执行，响应和 SSE 保留原结构。表内分别列出官方后端的支持情况和网页登录态 / OpenAI 兼容聊天的映射限制。配置与 SDK 示例见 [官方后端接入](/api/gemini-official)，内部能力见 [Gemini 参数与差距](/api/gemini)。', '',
  `共覆盖 **${used.size} 个 schema、${[...used].reduce((sum, name) => sum + rows(schemas[name]).length, 0)} 个字段定义**（共享字段按所属 schema 分别计数）、全部联合类型和枚举。自由对象的自定义键由调用方定义，不是可穷举的官方参数。`, '',
  '- [官方 OpenAPI 来源](https://ai.google.dev/static/api/interactions.openapi.json)',
  '- [官方 Markdown 来源](https://ai.google.dev/static/api/interactions.md.txt)',
  '- [保存的 OpenAPI 快照](/reference/gemini-interactions.openapi.json)',
  '- [保存的官方 Markdown 快照](/reference/gemini-interactions.official.txt)', '',
  '- [来源日期与校验值](/reference/gemini-sources.json)', '',
  `OpenAPI SHA-256：${code(sha256)}。`, '',
  '## 阅读规则', '',
  '字段是否必填、只读、废弃均按快照原样呈现；默认值仅在 schema 明确声明时填写。英文描述保留官方原文，中文用途见详解。schema 的 required 与“Output only”可能互相矛盾：尤其 ModelInteraction 的 created/id/status/updated 标为必填却是输出字段，steps 甚至在 required 中但不在 properties 中；这些字段不能据此加入创建请求。schema 与渲染版参考不一致时参阅差距页的待确认项。', '',
  '## 端点和路径 / 查询参数', '',
  '官方认证使用 x-goog-api-key；JSON 请求使用 Content-Type: application/json，SSE 可加 Accept: text/event-stream。这些头来自使用指南，不是请求体属性。Api-Revision 的历史变更见中文详解。', '',
]
for (const [pathname, item] of endpoints) {
  for (const method of methods) {
    const operation = item[method]
    if (!operation) continue
    lines.push(`### ${method.toUpperCase()} \`${pathname}\``, '', esc(operation.description), '',
      '| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |',
      '| --- | --- | --- | --- | --- | --- |')
    for (const parameter of [...(item.parameters ?? []), ...(operation.parameters ?? [])]) {
      const p = parameter.$ref ? spec.components.parameters[parameter.$ref.split('/').at(-1)] : parameter
      if (!p) throw new Error(`Unresolved parameter: ${parameter.$ref}`)
      lines.push(`| ${code(p.name)} | ${esc(p.in)} | ${type(p.schema ?? {})} | ${p.required ? '是' : '否'} | ${esc(p.description)} | 官方后端完整转发 |`)
    }
    lines.push('', `请求体：${operation.requestBody ? Object.values(operation.requestBody.content).map(c => type(c.schema)).join(' / ') : '无'}。`, '',
      `响应：${Object.entries(operation.responses).map(([http, response]) => `${code(http)} ${Object.entries(response.content ?? {}).map(([mime, c]) => `${code(mime)} → ${type(c.schema)}`).join('；') || esc(response.description)}`).join('；')}。`, '')
    for (const [http, response] of Object.entries(operation.responses)) {
      for (const [mime, content] of Object.entries(response.content ?? {})) {
        const inline = rows(content.schema ?? {})
        if (!inline.length) continue
        lines.push(`内联响应字段（${code(http)} ${code(mime)}）：`, '',
          '| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |',
          '| --- | --- | --- | --- | --- | --- |')
        for (const field of inline) lines.push(`| ${code(field.location)} | ${type(field.property)} | ${field.required ? '是' : '否'} | ${esc(field.property.description)} | 原样返回 | 不返回此官方包装 |`)
        lines.push('')
      }
    }
  }
}
for (const name of [...used].sort()) {
  const schema = schemas[name]
  lines.push(`## ${name} {#${anchor(name)}}`, '',
    `出现位置：${[input.has(name) && '请求体（可能同时用于输出）', output.has(name) && '响应 / 错误 / SSE'].filter(Boolean).join('；') || '官方保留组件'}。`, '',
    esc(schema.description), '', `结构 / 允许值：${type(schema)}。`, '')
  if (schema.discriminator) lines.push(`判别字段：${code(schema.discriminator.propertyName)}；映射：${code(JSON.stringify(schema.discriminator.mapping ?? {}))}。`, '')
  if (schema.required) lines.push(`官方 required：${schema.required.map(code).join('、')}。`, '')
  if (schema.deprecated) lines.push('此 schema 已废弃。', '')
  if (constraints(schema).length) lines.push(`对象约束 / 默认：${constraints(schema).map(code).join('；')}。`, '')
  if (Object.hasOwn(schema, 'additionalProperties')) lines.push(`自由属性：${code(JSON.stringify(schema.additionalProperties))}。`, '')
  const properties = rows(schema)
  if (properties.length) {
    lines.push('| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |',
      '| --- | --- | --- | --- | --- | --- | --- |')
    for (const { location, property, required } of properties) {
      const flags = [required ? '必填' : '可选', property.readOnly && '只读', property.writeOnly && '仅输入', property.deprecated && '废弃', /Output only/i.test(property.description ?? '') && '官方描述：仅输出'].filter(Boolean)
      const limits = constraints(property)
      lines.push(`| ${code(location)} | ${type(property)} | ${flags.join('；')} | ${limits.length ? limits.map(code).join('；') : '未注明'} | ${esc(property.description)} | 完整转发，由 Google 校验 | ${status(name, location)} |`)
    }
    lines.push('')
  }
  // Keep the meaning of enum values, including nested inline enum schemas.
  function enums(value, location = '$') {
    if (!value || typeof value !== 'object') return
    if (value.enum && value['x-google-enum-descriptions']) {
      lines.push(`枚举含义（${code(location)}）：`, '')
      value.enum.forEach((v, i) => lines.push(`- ${code(v)}：${esc(value['x-google-enum-descriptions'][i]) || '官方未说明'}`))
      lines.push('')
    }
    for (const [k, child] of Object.entries(value)) if (!['example', 'examples'].includes(k)) enums(child, `${location}.${k}`)
  }
  enums(schema)
}
const target = path.join(root, 'docs/api/gemini-schema.md')
const runtimeSnapshot = path.join(root, 'internal/geminiapi/official.openapi.json')
const generated = lines.join('\n').replace(/\n{3,}/g, '\n\n') + '\n'
if (process.argv.includes('--check')) {
  if (!fs.existsSync(runtimeSnapshot) || !fs.readFileSync(runtimeSnapshot).equals(raw)) throw new Error('Embedded official schema differs from the audited snapshot')
  if (!fs.existsSync(target) || fs.readFileSync(target, 'utf8').replaceAll('\r\n', '\n') !== generated) {
    throw new Error('Gemini inventory is stale. Run node scripts/audit-gemini-docs.mjs')
  }
} else {
  fs.writeFileSync(target, generated)
  fs.writeFileSync(runtimeSnapshot, raw)
}
console.log(`Gemini coverage: ${used.size} schemas, ${[...used].reduce((sum, n) => sum + rows(schemas[n]).length, 0)} fields, ${endpoints.length} paths; SHA-256 ${sha256}`)
