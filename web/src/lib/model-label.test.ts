import { test } from 'node:test'
import assert from 'node:assert/strict'
import { hasModelLabel, modelLabel } from './model-label.ts'

test('model label falls back to ID when display name is missing or blank', () => {
  for (const name of [undefined, '', '   ']) {
    assert.equal(modelLabel('gemini-2.5-flash', name), 'gemini-2.5-flash')
    assert.equal(hasModelLabel('gemini-2.5-flash', name), false)
  }
})

test('model label treats display names identical to the ID as absent', () => {
  for (const name of ['gemini-2.5-flash', ' models/gemini-2.5-flash ', 'MODELS/GEMINI-2.5-FLASH']) {
    assert.equal(modelLabel('gemini-2.5-flash', name), 'gemini-2.5-flash')
    assert.equal(hasModelLabel('gemini-2.5-flash', name), false)
  }
  // 缓存行存的是公开 ID，引擎上报的 display_name 可能带 models/ 前缀。
  assert.equal(modelLabel('models/gemini-2.5-flash', 'gemini-2.5-flash'), 'models/gemini-2.5-flash')
  assert.equal(hasModelLabel('models/gemini-2.5-flash', 'gemini-2.5-flash'), false)
})

test('model label keeps case-only humanized display names', () => {
  assert.equal(modelLabel('qwen3.5-397b', 'Qwen3.5-397B'), 'Qwen3.5-397B')
  assert.equal(hasModelLabel('qwen3.5-397b', 'Qwen3.5-397B'), true)
  assert.equal(modelLabel('deepseek-v4.1-flash', 'DeepSeek-V4.1-Flash'), 'DeepSeek-V4.1-Flash')
  assert.equal(hasModelLabel('deepseek-v4.1-flash', 'DeepSeek-V4.1-Flash'), true)
})

test('model label keeps a distinct display name', () => {
  assert.equal(modelLabel('gemini-2.5-flash', 'Gemini 2.5 Flash'), 'Gemini 2.5 Flash')
  assert.equal(hasModelLabel('gemini-2.5-flash', 'Gemini 2.5 Flash'), true)
  assert.equal(modelLabel('models/gemini-2.5-flash', 'Gemini 2.5 Flash'), 'Gemini 2.5 Flash')
})
