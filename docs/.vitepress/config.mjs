import { defineConfig } from 'vitepress'

const repository = process.env.GITHUB_REPOSITORY || ''
const [owner, name] = repository.split('/')
const base = process.env.GITHUB_ACTIONS && name ? `/${name}/` : '/'

export default defineConfig({
  lang: 'zh-CN',
  title: 'web2api',
  description: 'Gemini 双引擎号池管理网关与 OpenAI 兼容 API',
  base,
  cleanUrls: true,
  lastUpdated: true,
  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'web2api 文档',
    nav: [
      { text: '首页', link: '/' },
      { text: '快速开始', link: '/guide/quickstart' },
      { text: 'v1 API', link: '/api/overview' },
      { text: 'GitHub', link: `https://github.com/${owner && name ? `${owner}/${name}` : 'plunjoin/web2api'}` },
    ],
    sidebar: {
      '/guide/': [
        {
          text: '项目指南',
          items: [
            { text: '项目介绍', link: '/guide/introduction' },
            { text: '快速开始', link: '/guide/quickstart' },
            { text: '认证与 Key', link: '/guide/authentication' },
            { text: '部署方式', link: '/guide/deployment' },
          ],
        },
      ],
      '/api/': [
        {
          text: 'v1 API 文档',
          items: [
            { text: '接口总览', link: '/api/overview' },
            { text: '聊天补全', link: '/api/chat' },
            { text: '模型与状态', link: '/api/models' },
            { text: 'Veo 视频任务', link: '/api/videos' },
            { text: '图片生成', link: '/api/images' },
            { text: '音频生成', link: '/api/audio' },
            { text: '多模态透传', link: '/api/passthrough' },
            { text: 'Gemini 参数与差距', link: '/api/gemini' },
            { text: 'Gemini 官方后端接入', link: '/api/gemini-official' },
            { text: 'Gemini 官方字段完整对照', link: '/api/gemini-schema' },
            { text: 'Gemini 中文参考详解', link: '/api/gemini-reference' },
            { text: '错误与限流', link: '/api/errors' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: `https://github.com/${owner && name ? `${owner}/${name}` : 'plunjoin/web2api'}` },
    ],
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    footer: {
      message: '由 web2api 社区维护',
      copyright: 'MIT License',
    },
  },
})
