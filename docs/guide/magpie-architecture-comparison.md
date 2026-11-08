# Magpie 源码对照：借鉴哪些设计，不照搬哪些行为

## 参考范围

参考项目：[yetone/magpie](https://github.com/yetone/magpie)，固定版本为 [`c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee`](https://github.com/yetone/magpie/tree/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee)。结论来自该版本的关键源码及子系统文档，并非仅凭截图推断；没有构建或执行下载的参考代码，也没有验证真实上游账号。

本文不是“已移植 Magpie 路由”的声明。web2api 的实际实现与边界见 [账号池设计](./account-pool-design.md)。

## 1. WorkBuddy 登录失败不等于 Web 管理端不能授权

Magpie 的 [`internal/provider/workbuddy.go:3–11`](https://github.com/yetone/magpie/blob/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee/internal/provider/workbuddy.go#L3-L11) 明确说明：该版本 WorkBuddy / WorkBuddy AI 已由 `@magpie-community/opencode-workbuddy-auth` 插件服务，主仓库内建登录、请求和额度实现已移出。不能把主仓库残留的迁移代码当作当前授权协议的完整实现。

同一文件仍能确认两个发行版各自的 endpoint、platform 与 authID（第 46–70 行），以及账号身份不能按昵称去重（第 233–240 行）。这些桌面账号发现逻辑不能搬到远程 Web 服务上读取管理员电脑的凭据。

Magpie 的通用插件登录层 [`pluginsignin.go:117–161`](https://github.com/yetone/magpie/blob/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee/internal/provider/pluginsignin.go#L117-L161) 将授权 URL、验证码、粘贴回调和自动完成分开表达，并处理代理、取消和超时。但它不是 WorkBuddy 当前插件协议的替代证据，本次未验证那个插件包的实现。

web2api 的 WorkBuddy 流程是“服务端申请授权链接 → 管理员在浏览器授权 → 服务端轮询结果”，本身不需要管理员浏览器接收 localhost 回调。现有改动修正串行轮询、临时错误重试、过期重开、发行版/代理绑定和同一会话重复完成；不能据此宣称已在真实部署中成功登录。授权会话仍是单进程内存状态。

## 2. 额度卡片背后需要可靠的数据契约

Magpie 的 [`internal/gui/assets/quotaall.js:7–72`](https://github.com/yetone/magpie/blob/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee/internal/gui/assets/quotaall.js#L7-L72) 不只是画几个进度条：

- 绝对额度按 amount / limit 累加；钱包按 balance 累加；只有百分比时按相同套餐求平均。
- 聚合身份包含窗口名、family、pool、unit 和额度类型；百分比不能与积分混加。
- 失败是 unknown，不是余额为零；跨过重置的历史读数不能继续代表当前额度。
- 被更长周期耗尽所阻塞的短窗口，不被当作仍可花费的容量。
- 保留最早重置、参与账号数、未知账号数等解释信息。
- 只有两个以上匹配读数才显示该聚合窗口；不限量也不是只要有一个账号不限量就算全组不限量。

[`internal/provider/quotas.go:79–155`](https://github.com/yetone/magpie/blob/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee/internal/provider/quotas.go#L79-L155) 还区分共享余额、历史采样时间、余额单位、最早积分失效和文本窗口。UI 与路由应该消费这种结构化事实，而不是解析展示文案。

### web2api 已落实的子集

- 保留原额度文案，新增可选结构化快照；首批只接入 WorkBuddy 的绝对积分窗口。
- 国服、国际服分卡展示；分项和总计保留为独立窗口，不重复相加。
- 未知、失败、过期、停用、未就绪账号不伪装成可用余额。
- 有效期不只看 15 分钟：窗口重置或积分到期后，旧总计也不再用于汇总。历史数字保留，等待真实刷新，不自动补满、不估算扣减。
- 浏览器保留服务卡片折叠偏好，账号详情与检查动作分离。

### 有意不同的地方

当前标题是“可用账号额度”，只汇总已启用且就绪账号；不是 Magpie 的全部可读账号资产汇总。没有实现 family / pool、百分比套餐平均、钱包聚合、不限量、跨周期阻塞等完整语义。尤其第三方多个 Key 可能共享同一钱包，没有计费身份时不跨 Key 加钱。

## 3. Magpie 的路由是多个阶段，不是一个全局分数

以下策略结论来自实际读取的 [`internal/provider/routing.go`](https://github.com/yetone/magpie/blob/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee/internal/provider/routing.go) 和 [`internal/routes/strategies.go`](https://github.com/yetone/magpie/blob/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee/internal/routes/strategies.go)，不是只依据策略名称：

| 策略 | 源码中的关键语义 | 不能误解为 |
| --- | --- | --- |
| smart | 先分正常、低额、将耗尽；正常候选比较长窗口的重置时间、后续窗口、积分失效、可比钱包等（strategies.go 第 40–98、142–194 行） | 简单选择使用最少的账号 |
| pace | 以剩余比例和距重置小时计算消耗紧迫度，多预算窗口取最紧约束；排序另有分带和余额等比较（routing.go 第 727–798 行；strategies.go 第 395–447 行） | 比较所有账号的原始余额 |
| usage | 区分已知可用、未知、低额、耗尽等状态，另有未知账号学习例外；档内比较已用比例、可比余额、近期用量（strategies.go 第 317–371 行） | 把未知量算作零后统一排序 |
| rotate | 按池计数轮换首选候选（strategies.go 第 297–304 行） | 每一请求必定换号，不考虑会话 |

额度评估按当前模型选择适用窗口，不能拿不相关模型的额度做排名。积分失效时间与周期重置不同；钱包也只能在匹配供应商和单位时比较（routing.go 第 427–449、538–616 行）。

### 文档说明但未逐项验证网关实码的部分

[`docs/subsystems/gateway-routing.md`](https://github.com/yetone/magpie/blob/c6bdc172a01e1eb8713c596e33e1ef703eb9b4ee/docs/subsystems/gateway-routing.md) 还说明了能力过滤、账号 pin、上下文容量、affinity 模式、候选 fallback、已经输出后的重试边界，以及实际尝试 trace。本次没有下载并核对这些行为涉及的全部网关实现，因此不把文档描述写成已验证的执行保证。

其中适合作为设计约束的是：资格过滤、策略排序、会话保持、实际发送与失败处理分别负责；候选排序位置不等于已经尝试过的账号；向客户端发出回复后，不能再换号拼接另一段回复。

## 4. web2api 的移植边界

web2api 是多引擎号池和 Key 分发网关，必须保留“账号目录明确声明模型才可认领”的规则。服务卡片是展示分组，不自动形成独立的生产调度池；裸模型 ID 的开放设置也仍然是全局的。

这轮没有新增 smart / pace / usage 策略，没有把所有供应商百分比混成一个评分，也没有替换全局默认路由。WorkBuddy 已有的“按模型及发行版权益过滤 → 优先消耗最早失效积分 → 无到期信息时轮换”仍保留。

已落实的路由相关改进是**定向探测隔离**：指定 engine / custom provider 的模型测试必须由目标执行，不允许另一供应商的同名模型代答。目标不可用就返回目标错误；未指定目标的旧入口保留兼容行为。WorkBuddy 的“引擎模型”仍是双发行版共享目录，不宣称按服务卡片独立探测。

这套取舍借鉴的是数据和职责边界，而不是把桌面产品的所有行为搬进服务端，也不把展示层改版误称为生产调度器重写。
