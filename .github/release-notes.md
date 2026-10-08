v0.2.6 修复升级到 v0.2.5 后管理台没有样式、版本显示为 dev 的问题。

- 原因：v0.2.5 改了管理台页面，样式表地址仍是 /admin/assets/admin.css，且静态资源没有任何缓存校验头。浏览器或反向代理/CDN 里缓存的 v0.2.4 样式表被拿来配新页面，新页面用到的样式类旧文件里都没有，于是整页失去布局。镜像本身返回的样式是正确的。
- 管理台资源地址现在带内容哈希（例如 /admin/assets/admin.css?v=c5953797b0d7），每次发版自动换新地址，旧缓存不会再与新页面混用。带哈希的地址可长期缓存；不带哈希的地址要求每次校验（ETag，未变化返回 304）。页面本身仍为 no-store。
- 版本号：容器构建未传 VERSION 时不再显示 dev，改为读取随源码发布的 internal/version/VERSION（本版为 v0.2.6）。发布包仍由构建参数注入版本号。

升级后无需改配置，数据库不变。若前面有 CDN（如 Cloudflare）且曾缓存过旧样式，新版本已绕开该缓存；如仍异常，清一次 CDN 缓存或强制刷新浏览器即可。

容器镜像：ghcr.io/plunjoin/web2api:v0.2.6、ghcr.io/plunjoin/web2api:0.2.6、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：Go 全量测试；新增资源哈希、缓存头、304 校验和版本回退单测；按 .dockerignore 构建上下文（无 .git、VERSION=dev）编译的二进制返回 v0.2.6，/admin 链接的样式表返回 200 text/css；模拟浏览器持有旧样式缓存时页面仍为完整深色布局。
