function isModelIDAlias(name: string, id: string) {
  const bare = id.trim()
  return name === bare || name.toLowerCase() === `models/${bare.toLowerCase()}` || `models/${name.toLowerCase()}` === bare.toLowerCase()
}

/** 模型列表的主标题：优先用 display_name，空值或等于 ID 时回退到 ID。 */
export function modelLabel(id: string, displayName?: string) {
  const name = displayName?.trim() || ''
  if (!name || isModelIDAlias(name, id)) return id
  return name
}

/** 是否还有不同于 ID 的展示名，用来决定是否显示 ID 副标题。 */
export function hasModelLabel(id: string, displayName?: string) {
  return modelLabel(id, displayName) !== id
}
