import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import readline from "node:readline";
import { chromium } from "playwright";

const apiBase = String(process.env.WEB2API_URL || "").replace(/\/+$/, "");
const token = String(process.env.WEB2API_ADMIN_JWT || "").trim();
const email = String(process.env.WEB2API_EMAIL || "").trim();
const label = String(process.env.WEB2API_LABEL || email).trim();
if (!apiBase || !token || !email) {
  throw new Error("WEB2API_URL、WEB2API_ADMIN_JWT、WEB2API_EMAIL 均不能为空");
}

function waitForEnter() {
  const ttyPath = process.env.WEB2API_TTY;
  const input = ttyPath && fs.existsSync(ttyPath) ? fs.createReadStream(ttyPath) : process.stdin;
  return new Promise((resolve) => {
    const rl = readline.createInterface({ input, output: process.stdout });
    rl.question("\n登录完成后按 Enter 导出登录态：", () => {
      rl.close();
      if (input !== process.stdin) input.close();
      resolve();
    });
  });
}

const profileDir = fs.mkdtempSync(path.join(os.tmpdir(), "web2api-aistudio-"));
let context;
try {
  // 优先使用本机正式 Chrome，降低 Google 对自动化浏览器的拦截概率。
  context = await chromium.launchPersistentContext(profileDir, {
    channel: "chrome",
    headless: false,
    args: ["--disable-blink-features=AutomationControlled"],
  });
} catch {
  context = await chromium.launchPersistentContext(profileDir, {
    headless: false,
    args: ["--disable-blink-features=AutomationControlled"],
  });
}

try {
  const page = context.pages()[0] || await context.newPage();
  await page.goto("https://aistudio.google.com/", { waitUntil: "domcontentloaded", timeout: 120000 });
  console.log("已打开 AI Studio。请在弹出的浏览器中完成 Google 登录，并确认能看到 AI Studio 页面。");
  await waitForEnter();
  const state = await context.storageState();
  const response = await fetch(`${apiBase}/admin/api/accounts`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      engine: "b",
      email,
      label,
      storage_state: JSON.stringify(state),
    }),
  });
  const body = await response.text();
  if (!response.ok) {
    throw new Error(`后端添加账号失败（HTTP ${response.status}）：${body}`);
  }
  console.log("账号已添加到后端：");
  console.log(body);
} finally {
  await context.close();
  fs.rmSync(profileDir, { recursive: true, force: true });
}
