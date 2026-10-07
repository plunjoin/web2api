# Gemini 官方字段完整对照

> 此页由 `scripts/audit-gemini-docs.mjs` 从官方 OpenAPI 快照生成。请通过修改快照并重新生成来更新此页。

对照日期：2026-10-07（北京时间）。范围为保存的官方 OpenAPI 中全部 20 个路径、37 个操作和全部组件：Interactions 创建 / 获取 / 取消 / 删除、agents、voices、environments、webhooks、triggers、credentials 等资源，以及请求、响应、错误和 SSE schema。快照未定义的独立 Files / models API 通过官方后端通用路由转发，使用流程见中文参考详解。

启用 gemini_api 后，官方路由完整转发本页全部字段、联合类型及未来扩展字段，由 Google 校验并执行，响应和 SSE 保留原结构。表内分别列出官方后端的支持情况和网页登录态 / OpenAI 兼容聊天的映射限制。配置与 SDK 示例见 [官方后端接入](/api/gemini-official)，内部能力见 [Gemini 参数与差距](/api/gemini)。

共覆盖 **214 个 schema、722 个字段定义**（共享字段按所属 schema 分别计数）、全部联合类型和枚举。自由对象的自定义键由调用方定义，不是可穷举的官方参数。

- [官方 OpenAPI 来源](https://ai.google.dev/static/api/interactions.openapi.json)
- [官方 Markdown 来源](https://ai.google.dev/static/api/interactions.md.txt)
- [保存的 OpenAPI 快照](/reference/gemini-interactions.openapi.json)
- [保存的官方 Markdown 快照](/reference/gemini-interactions.official.txt)

- [来源日期与校验值](/reference/gemini-sources.json)

OpenAPI SHA-256：<code>6f3d064c9b711b84e8b71a9a03533aaf1f8ed006adcf7697ffacf8c192baa419</code>。

## 阅读规则

字段是否必填、只读、废弃均按快照原样呈现；默认值仅在 schema 明确声明时填写。英文描述保留官方原文，中文用途见详解。schema 的 required 与“Output only”可能互相矛盾：尤其 ModelInteraction 的 created/id/status/updated 标为必填却是输出字段，steps 甚至在 required 中但不在 properties 中；这些字段不能据此加入创建请求。schema 与渲染版参考不一致时参阅差距页的待确认项。

## 端点和路径 / 查询参数

官方认证使用 x-goog-api-key；JSON 请求使用 Content-Type: application/json，SSE 可加 Accept: text/event-stream。这些头来自使用指南，不是请求体属性。Api-Revision 的历史变更见中文详解。

### PUT `/upload/{api_version}/environments/{environment}/files/{path}`

Starts a resumable upload session for a file in an environment workspace. Upload the file bytes to the URL returned in the `X-Goog-Upload-URL` response header, using the resumable upload protocol.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>environment</code> | path | <code>string</code> | 是 | The ID of the environment that owns the destination file. | 官方后端完整转发 |
| <code>path</code> | path | <code>string</code> | 是 | The relative destination path inside the environment workspace. | 官方后端完整转发 |
| <code>extract</code> | query | <code>boolean</code> | 否 | Optional. If true, treats the uploaded file as a tar/tar.gz archive and unpacks it into `path`. | 官方后端完整转发 |
| <code>overwrite</code> | query | <code>boolean</code> | 否 | Optional. Whether to overwrite the destination file if it already exists. | 官方后端完整转发 |
| <code>X-Goog-Upload-Command</code> | header | <code>"start"</code> | 是 | Command that starts the resumable upload session. | 官方后端完整转发 |
| <code>X-Goog-Upload-Header-Content-Length</code> | header | <code>integer (int64)</code> | 是 | Total number of file bytes that will be uploaded to the session URL. | 官方后端完整转发 |
| <code>X-Goog-Upload-Header-Content-Type</code> | header | <code>string</code> | 是 | MIME type of the file that will be uploaded to the session URL. | 官方后端完整转发 |
| <code>X-Goog-Upload-Protocol</code> | header | <code>"resumable"</code> | 是 | Resumable upload protocol selector. | 官方后端完整转发 |

请求体：无。

响应：<code>200</code> Upload session created successfully. Send the file bytes to the returned upload URL.；<code>4XX</code> The upload session request failed with a client error.；<code>5XX</code> The upload session request failed with a server error.。

### GET `/{api_version}/agents`

Lists all Agents.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 |  | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 |  | 官方后端完整转发 |
| <code>parent</code> | query | <code>string</code> | 否 | Required. The parent resource to list agents from. Format: `projects/&#123;project&#125;/locations/&#123;location&#125;` | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [ListAgentsResponse](#schema-listagentsresponse)。

### POST `/{api_version}/agents`

Creates a new Agent (Typed version for SDK).

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |

请求体：[Agent](#schema-agent)。

响应：<code>default</code> <code>application/json</code> → [Agent](#schema-agent)。

### GET `/{api_version}/agents/{agentsId}`

Gets a specific Agent.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>agentsId</code> | path | <code>string</code> | 是 | Required. The name of the agent to retrieve. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Agent](#schema-agent)。

### DELETE `/{api_version}/agents/{agentsId}`

Deletes an Agent.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>agentsId</code> | path | <code>string</code> | 是 | Required. The name of the agent to delete. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Empty](#schema-empty)。

### GET `/{api_version}/credentials`

Lists credentials.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 | Optional. Maximum number of credentials to return. | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 | Optional. Pagination token. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [ListCredentialsResponse](#schema-listcredentialsresponse)。

### POST `/{api_version}/credentials`

Creates a new credential.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |

请求体：[CredentialCreateParams](#schema-credentialcreateparams)。

响应：<code>default</code> <code>application/json</code> → [Credential](#schema-credential)。

### GET `/{api_version}/credentials/{id}`

Gets a credential by ID.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource ID segment making up resource `name`. It identifies the resource within its parent collection as described in https://google.aip.dev/122. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Credential](#schema-credential)。

### PATCH `/{api_version}/credentials/{id}`

Updates a credential.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource ID segment making up resource `name`. It identifies the resource within its parent collection as described in https://google.aip.dev/122. | 官方后端完整转发 |
| <code>update_mask</code> | query | <code>string</code> | 否 | Optional. The list of fields to update. | 官方后端完整转发 |

请求体：[CredentialUpdateParams](#schema-credentialupdateparams)。

响应：<code>default</code> <code>application/json</code> → [Credential](#schema-credential)。

### DELETE `/{api_version}/credentials/{id}`

Deletes a credential.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource ID segment making up resource `name`. It identifies the resource within its parent collection as described in https://google.aip.dev/122. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Empty](#schema-empty)。

### GET `/{api_version}/environments`

Lists environments.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 | Optional. Maximum number of environments to return. If unspecified, defaults to 50. Maximum is 1000. | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 | Optional. Pagination token. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [ListEnvironmentsResponse](#schema-listenvironmentsresponse)。

### POST `/{api_version}/environments`

Creates an environment.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |

请求体：[CreateEnvironmentRequest](#schema-createenvironmentrequest)。

响应：<code>default</code> <code>application/json</code> → [Environment](#schema-environment)。

### GET `/{api_version}/environments/{environment}/files/{path}`

Retrieves file metadata or directory contents from an environment's snapshot. To download file content, use the download URL returned in the response.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>environment</code> | path | <code>string</code> | 是 | The ID of the environment whose snapshot to read. | 官方后端完整转发 |
| <code>path</code> | path | <code>string</code> | 是 | Path of the file or directory inside the environment workspace, relative to its root (e.g. src). | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 | Optional. Maximum number of entries to return per page (for directory listing). | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 | Optional. Pagination token for directory listing. | 官方后端完整转发 |
| <code>recursive</code> | query | <code>boolean</code> | 否 | Optional. If true and the path is a directory, recursively lists all files. | 官方后端完整转发 |

请求体：无。

响应：<code>200</code> <code>application/json</code> → [GetEnvironmentFilesResponse](#schema-getenvironmentfilesresponse)。

### GET `/{api_version}/environments/{id}`

Gets an environment.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource ID segment making up resource `name`. It identifies the resource within its parent collection as described in https://google.aip.dev/122. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Environment](#schema-environment)。

### DELETE `/{api_version}/environments/{id}`

Deletes an environment.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource ID segment making up resource `name`. It identifies the resource within its parent collection as described in https://google.aip.dev/122. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Empty](#schema-empty)。

### POST `/{api_version}/interactions`

Creates a new interaction.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |

请求体：[AgentInteraction](#schema-agentinteraction) / [ModelInteraction](#schema-modelinteraction)。

响应：<code>200</code> <code>application/json</code> → [Interaction](#schema-interaction)；<code>text/event-stream</code> → [InteractionSseStreamEnvelope](#schema-interactionssestreamenvelope)；<code>4XX</code> <code>application/json</code> → <code>object</code>；<code>5XX</code> <code>application/json</code> → <code>object</code>。

内联响应字段（<code>4XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

内联响应字段（<code>5XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

### GET `/{api_version}/interactions/{interactionsId}`

Retrieves the full details of a single interaction based on its `Interaction.id`.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>include_input</code> | query | <code>boolean</code> | 否 | If true, includes the input in the response. | 官方后端完整转发 |
| <code>interactionsId</code> | path | <code>string</code> | 是 | Required. The name of the interaction to retrieve. | 官方后端完整转发 |
| <code>last_event_id</code> | query | <code>string</code> | 否 | If set, resumes the interaction stream from the chunk after the event marked by the event id. Can only be used if `stream` is true. | 官方后端完整转发 |
| <code>stream</code> | query | <code>boolean</code> | 否 | If true, streams the interaction events as Server-Sent Events. | 官方后端完整转发 |

请求体：无。

响应：<code>200</code> <code>application/json</code> → [Interaction](#schema-interaction)；<code>text/event-stream</code> → [InteractionSseStreamEnvelope](#schema-interactionssestreamenvelope)；<code>4XX</code> <code>application/json</code> → <code>object</code>；<code>5XX</code> <code>application/json</code> → <code>object</code>。

内联响应字段（<code>4XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

内联响应字段（<code>5XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

### DELETE `/{api_version}/interactions/{interactionsId}`

Deletes the interaction by id.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>interactionsId</code> | path | <code>string</code> | 是 | Required. The name of the interaction to delete. | 官方后端完整转发 |

请求体：无。

响应：<code>200</code> Successful deletion of the interaction.；<code>4XX</code> <code>application/json</code> → <code>object</code>；<code>5XX</code> <code>application/json</code> → <code>object</code>。

内联响应字段（<code>4XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

内联响应字段（<code>5XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

### POST `/{api_version}/interactions/{interactionsId}/cancel`

Cancels an interaction by id. This only applies to background interactions that are still running.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>interactionsId</code> | path | <code>string</code> | 是 | Required. The name of the interaction to cancel. | 官方后端完整转发 |

请求体：无。

响应：<code>200</code> <code>application/json</code> → [Interaction](#schema-interaction)；<code>4XX</code> <code>application/json</code> → <code>object</code>；<code>5XX</code> <code>application/json</code> → <code>object</code>。

内联响应字段（<code>4XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

内联响应字段（<code>5XX</code> <code>application/json</code>）：

| 字段 | 类型 | 必填 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 是 |  | 原样返回 | 不返回此官方包装 |

### GET `/{api_version}/triggers`

Lists triggers for a project.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>filter</code> | query | <code>string</code> | 否 | Optional. Filter expression (e.g., by state). | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 | Optional. The maximum number of triggers to return per page. | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 | Optional. A page token from a previous ListTriggers call. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [ListTriggersResponse](#schema-listtriggersresponse)。

### POST `/{api_version}/triggers`

Creates a new trigger that will invoke the specified agent on the given cron schedule.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |

请求体：[TriggerCreateParams](#schema-triggercreateparams)。

响应：<code>default</code> <code>application/json</code> → [Trigger](#schema-trigger)。

### GET `/{api_version}/triggers/{id}`

Gets details of a single trigger.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource name of the trigger. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Trigger](#schema-trigger)。

### PATCH `/{api_version}/triggers/{id}`

Updates a trigger.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource name of the trigger. | 官方后端完整转发 |

请求体：[TriggerUpdate](#schema-triggerupdate)。

响应：<code>default</code> <code>application/json</code> → [Trigger](#schema-trigger)。

### DELETE `/{api_version}/triggers/{id}`

Deletes a trigger.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. Resource name of the trigger. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Empty](#schema-empty)。

### GET `/{api_version}/triggers/{triggerId}/executions`

Lists executions for a trigger.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 | The maximum number of executions to return per page. | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 | A page token from a previous ListTriggerExecutions call. | 官方后端完整转发 |
| <code>triggerId</code> | path | <code>string</code> | 是 | Required. The trigger ID to list executions from. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [ListTriggerExecutionsResponse](#schema-listtriggerexecutionsresponse)。

### POST `/{api_version}/triggers/{triggerId}/executions`

Runs a trigger immediately.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>triggerId</code> | path | <code>string</code> | 是 | Required. Resource name of the trigger. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [TriggerExecution](#schema-triggerexecution)。

### GET `/{api_version}/voices`

Lists custom stored voices owned by the caller (ordered newest first) followed by prebuilt system voices from Google's voice catalog.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>accent</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by accent description (e.g. "American", "British"). Case-insensitive exact match. If multiple values are specified, matches voices with any of the specified accents (OR). | 官方后端完整转发 |
| <code>context</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by intended context or domain (e.g. "News, Commercial"). Case-insensitive exact match. If multiple values are specified, matches voices with any of the specified contexts (OR). | 官方后端完整转发 |
| <code>gender</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by gender presentation (e.g. "female", "male", "neutral"). Case-insensitive exact match. If multiple values are specified, matches voices with any of the specified genders (OR). | 官方后端完整转发 |
| <code>language_code</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by BCP-47 language code (e.g. "en-US"). Case-insensitive exact match. If multiple values are specified, matches voices with any of the specified language codes (OR). | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 | Optional. The maximum number of voices to return per page. The service may return fewer than this value. If unspecified, at most 50 voices are returned. The maximum value is 1000; values above 1000 are coerced to 1000. | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 | Optional. A page token received from a previous `ListVoices` call. Provide this to retrieve the subsequent page.  When paginating, all filter query parameters (`language_code`, `region_code`, `accent`, `persona`, `context`, `gender`, `pitch`, `type`, and `search`) must match the call that returned this token; otherwise the request fails with `INVALID_ARGUMENT`. `page_size` may change between pages. | 官方后端完整转发 |
| <code>persona</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by vocal persona (e.g. "Warm, Friendly"). Case-insensitive exact match. If multiple values are specified, matches voices with any of the specified personas (OR). | 官方后端完整转发 |
| <code>pitch</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by vocal pitch. Accepts `"low"`, `"medium"`, `"high"` (case-insensitive). If multiple values are specified, matches voices with any of the specified pitches (OR). | 官方后端完整转发 |
| <code>region_code</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by ISO 3166-1 alpha-2 or UN M.49 region code (e.g. "US", "001"). Case-insensitive exact match. If multiple values are specified, matches voices with any of the specified region codes (OR). | 官方后端完整转发 |
| <code>search</code> | query | <code>string</code> | 否 | Optional. Free-text substring search query matched case-insensitively against both `display_name` and `description`. Maximum 2048 bytes. | 官方后端完整转发 |
| <code>type</code> | query | Array&lt;<code>string</code>&gt; | 否 | Optional. Filter by voice type. Accepts `"prebuilt"`, `"replicated"`, `"prompted"` (case-insensitive). If multiple values are specified, matches voices with any of the specified types (OR). | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [ListVoicesResponse](#schema-listvoicesresponse)。

### POST `/{api_version}/voices`

Creates a custom voice from a natural-language prompt (`VOICE_TYPE_PROMPTED`) or from reference and consent audio recordings (`VOICE_TYPE_REPLICATED`).

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |

请求体：[CreateVoiceRequest](#schema-createvoicerequest)。

响应：<code>default</code> <code>application/json</code> → [Voice](#schema-voice)。

### GET `/{api_version}/voices/{voicesId}`

Gets a custom stored voice (`store = true`) by resource name. Prebuilt catalog voices (`VOICE_TYPE_PREBUILT`) cannot be retrieved via `GetVoice`; use `ListVoices` instead.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>voicesId</code> | path | <code>string</code> | 是 | Required. The resource name of the custom stored voice to retrieve (for example, `voices/voice_abc123def456`). | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Voice](#schema-voice)。

### DELETE `/{api_version}/voices/{voicesId}`

Deletes a custom stored voice (`store = true`) by resource name. Prebuilt catalog voices (`VOICE_TYPE_PREBUILT`) cannot be deleted.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>voicesId</code> | path | <code>string</code> | 是 | Required. The resource name of the custom stored voice to delete (for example, `voices/voice_abc123def456`). | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [DeleteVoiceResponse](#schema-deletevoiceresponse)。

### GET `/{api_version}/webhooks`

Lists all Webhooks.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>page_size</code> | query | <code>integer (int32)</code> | 否 | Optional. The maximum number of webhooks to return. The service may return fewer than this value. If unspecified, at most 50 webhooks will be returned. The maximum value is 1000. | 官方后端完整转发 |
| <code>page_token</code> | query | <code>string</code> | 否 | Optional. A page token, received from a previous `ListWebhooks` call. Provide this to retrieve the subsequent page. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [ListWebhooksResponse](#schema-listwebhooksresponse)。

### POST `/{api_version}/webhooks`

Creates a new Webhook.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |

请求体：[Webhook](#schema-webhook)。

响应：<code>default</code> <code>application/json</code> → [Webhook](#schema-webhook)。

### GET `/{api_version}/webhooks/{id}`

Gets a specific Webhook.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. The ID of the webhook to retrieve. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Webhook](#schema-webhook)。

### PATCH `/{api_version}/webhooks/{id}`

Updates an existing Webhook.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. The ID of the webhook to update. | 官方后端完整转发 |
| <code>update_mask</code> | query | <code>string</code> | 否 | Optional list of fields to update. | 官方后端完整转发 |

请求体：[WebhookUpdate](#schema-webhookupdate)。

响应：<code>default</code> <code>application/json</code> → [Webhook](#schema-webhook)。

### DELETE `/{api_version}/webhooks/{id}`

Deletes a Webhook.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. The ID of the webhook to delete. | 官方后端完整转发 |

请求体：无。

响应：<code>default</code> <code>application/json</code> → [Empty](#schema-empty)。

### POST `/{api_version}/webhooks/{id}:ping`

Sends a ping event to a Webhook.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. The ID of the webhook to ping. | 官方后端完整转发 |

请求体：[PingWebhookRequest](#schema-pingwebhookrequest)。

响应：<code>default</code> <code>application/json</code> → [PingWebhookResponse](#schema-pingwebhookresponse)。

### POST `/{api_version}/webhooks/{id}:rotateSigningSecret`

Generates a new signing secret for a Webhook.

| 参数 | 位置 | 类型 / 允许值 | 必填 | 官方说明 | web2api |
| --- | --- | --- | --- | --- | --- |
| <code>api_version</code> | path | <code>string</code> | 是 | API version for request routing. | 官方后端完整转发 |
| <code>id</code> | path | <code>string</code> | 是 | Required. The ID of the webhook for which to generate a signing secret. | 官方后端完整转发 |

请求体：[RotateSigningSecretRequest](#schema-rotatesigningsecretrequest)。

响应：<code>default</code> <code>application/json</code> → [RotateSigningSecretResponse](#schema-rotatesigningsecretresponse)。

## Agent {#schema-agent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

An agent definition for the CreateAgent API. This message is the target for annotation-parser-based JSON parsing. New format:   &#123;     "id": "customer-sentinel",     "base_agent": "",     "system_instruction": "...",     "base_environment": &#123; "type": "remote", "sources": [...] &#125;,     "tools": [ &#123;"type": "code_execution"&#125; ]   &#125;

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>agent_config</code> | [AntigravityAgentConfig](#schema-antigravityagentconfig) | 可选 | 未注明 | Configuration parameters for the agent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>base_agent</code> | <code>string</code> | 可选 | 未注明 | The base agent to extend. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>base_environment</code> | [EnvironmentConfig](#schema-environmentconfig) / <code>string</code> | 可选 | 未注明 | The environment configuration for the agent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>description</code> | <code>string</code> | 可选 | 未注明 | Agent description for developers to quickly read and understand. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 可选 | 未注明 | The unique identifier for the agent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>system_instruction</code> | <code>string</code> | 可选 | 未注明 | System instruction for the agent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>tools</code> | Array&lt;[AgentTool](#schema-agenttool)&gt; | 可选 | 未注明 | The tools available to the agent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## AgentInteraction {#schema-agentinteraction}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Interaction for generating the completion using agents.

结构 / 允许值：<code>object</code>。

官方 required：<code>agent</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>agent</code> | [AgentOption](#schema-agentoption) | 必填 | 未注明 | The name of the `Agent` used for generating the interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>agent_config</code> | [AntigravityAgentConfig](#schema-antigravityagentconfig) / [CodeMenderAgentConfig](#schema-codemenderagentconfig) / [DeepResearchAgentConfig](#schema-deepresearchagentconfig) / [DynamicAgentConfig](#schema-dynamicagentconfig) | 可选 | 未注明 | Configuration parameters for the agent interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>background</code> | <code>boolean</code> | 可选 | 未注明 | Input only. Whether to run the model interaction in the background. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>continuation_token</code> | <code>string (byte)</code> | 可选 | 未注明 | Opaque token to resume a long decode. Output: set when status is INCOMPLETE and decoding can be resumed. Input: pass the latest token back unchanged in CreateInteraction to continue decoding. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>environment</code> | [EnvironmentConfig](#schema-environmentconfig) / <code>string</code> | 可选 | 未注明 | The environment configuration for the interaction. Can be an object specifying remote environment sources or a string referencing an existing environment ID. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>input</code> | [InteractionsInput](#schema-interactionsinput) | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 部分：messages 仅文本；没有 Content/Step JSON 接口 |
| <code>labels</code> | Map&lt;string, <code>string</code>&gt; | 可选 | 未注明 | The labels with user-defined metadata for the request.  Label keys and values can be no longer than 63 characters (Unicode codepoints) and can only contain lowercase letters, numeric characters, underscores, and dashes. International characters are allowed. Label values are optional. Label keys must start with a letter. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>previous_interaction_id</code> | <code>string</code> | 可选 | 未注明 | The ID of the previous interaction, if any. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>response_format</code> | [ResponseFormat](#schema-responseformat) / Array&lt;[ResponseFormat](#schema-responseformat)&gt; | 可选 | 未注明 | Enforces that the generated response is a JSON object that complies with the JSON schema specified in this field. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>response_mime_type</code> | <code>string</code> | 可选；废弃 | 未注明 | The mime type of the response. This is required if response_format is set. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>response_modalities</code> | Array&lt;[ResponseModality](#schema-responsemodality)&gt; | 可选；废弃 | 未注明 | The requested modalities of the response (TEXT, IMAGE, AUDIO). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>safety_settings</code> | Array&lt;[SafetySetting](#schema-safetysetting)&gt; | 可选 | 未注明 | Safety settings for the interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>service_tier</code> | [ServiceTier](#schema-servicetier) | 可选 | 未注明 | The service tier for the interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>store</code> | <code>boolean</code> | 可选 | 未注明 | Input only. Whether to store the response and request for later retrieval. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>stream</code> | <code>boolean</code> | 可选 | 未注明 | Input only. Whether the interaction will be streamed. | 完整转发，由 Google 校验 | 部分：聊天 SSE；不是 Interactions 事件流 |
| <code>system_instruction</code> | <code>string</code> | 可选 | 未注明 | System instruction for the interaction. | 完整转发，由 Google 校验 | 映射：messages 中的 system 文本 |
| <code>tools</code> | Array&lt;[Tool](#schema-tool)&gt; | 可选 | 未注明 | A list of tool declarations the model may call during interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>webhook_config</code> | [WebhookConfig](#schema-webhookconfig) | 可选 | 未注明 | Optional. Webhook configuration for receiving notifications when the interaction completes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## AgentOption {#schema-agentoption}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The agent to interact with.

结构 / 允许值：<code>"deep-research-pro-preview-12-2025"</code> / <code>"deep-research-preview-04-2026"</code> / <code>"deep-research-max-preview-04-2026"</code> / <code>"antigravity-preview-05-2026"</code>。

## AgentTool {#schema-agenttool}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that the agent can use.

结构 / 允许值：[CodeExecution](#schema-codeexecution) / [Function](#schema-function) / [GoogleSearch](#schema-googlesearch) / [McpServer](#schema-mcpserver) / [UrlContext](#schema-urlcontext)。

## AllowedTools {#schema-allowedtools}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The configuration for allowed tools.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>mode</code> | <code>"auto"</code> / <code>"any"</code> / <code>"none"</code> / <code>"validated"</code> | 可选 | 未注明 | The mode of the tool choice. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>tools</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | The names of the allowed tools. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mode</code>）：

- <code>auto</code>：Auto tool choice.
- <code>any</code>：Any tool choice.
- <code>none</code>：No tool choice.
- <code>validated</code>：Validated tool choice.

## Annotation {#schema-annotation}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Citation information for model-generated content.

结构 / 允许值：[FileCitation](#schema-filecitation) / [PlaceCitation](#schema-placecitation) / [SpeechAnnotation](#schema-speechannotation) / [UrlCitation](#schema-urlcitation) / [WordInfo](#schema-wordinfo)。

## AntigravityAgentConfig {#schema-antigravityagentconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for the Antigravity agent runtime. Provides server-side control over the agent's execution environment and tool configuration.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>max_total_tokens</code> | <code>string (int64)</code> | 可选 | 未注明 | Max total tokens for the agent run. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>model</code> | <code>string</code> | 可选 | 未注明 | The model to use for agent reasoning. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"antigravity"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ArgumentsDelta {#schema-argumentsdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"arguments_delta"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## AudioContent {#schema-audiocontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

An audio content block.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>channels</code> | <code>integer (int32)</code> | 可选 | 未注明 | The number of audio channels. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 | The audio content. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mime_type</code> | <code>"audio/wav"</code> / <code>"audio/mp3"</code> / <code>"audio/aiff"</code> / <code>"audio/aac"</code> / <code>"audio/ogg"</code> / <code>"audio/flac"</code> / <code>"audio/mpeg"</code> / <code>"audio/m4a"</code> / <code>"audio/l16"</code> / <code>"audio/opus"</code> / <code>"audio/alaw"</code> / <code>"audio/mulaw"</code> / <code>"audio/webm"</code> | 可选 | 未注明 | The mime type of the audio. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>sample_rate</code> | <code>integer (int32)</code> | 可选 | 未注明 | The sample rate of the audio. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"audio"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 | The URI of the audio. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>audio/wav</code>：WAV audio format
- <code>audio/mp3</code>：MP3 audio format
- <code>audio/aiff</code>：AIFF audio format
- <code>audio/aac</code>：AAC audio format
- <code>audio/ogg</code>：OGG audio format
- <code>audio/flac</code>：FLAC audio format
- <code>audio/mpeg</code>：MPEG audio format
- <code>audio/m4a</code>：M4A audio format
- <code>audio/l16</code>：L16 audio format
- <code>audio/opus</code>：OPUS audio format
- <code>audio/alaw</code>：ALAW audio format
- <code>audio/mulaw</code>：MULAW audio format
- <code>audio/webm</code>：WebM audio format

## AudioData {#schema-audiodata}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Audio payload used for voice creation.

结构 / 允许值：<code>object</code>。

官方 required：<code>data</code>、<code>mime_type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | <code>string (byte)</code> | 必填 | 未注明 | Required. The raw audio bytes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mime_type</code> | <code>string</code> | 必填 | 未注明 | Required. The IANA MIME type of the audio data (for example, `audio/wav` or `audio/mpeg`). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## AudioDelta {#schema-audiodelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>channels</code> | <code>integer (int32)</code> | 可选 | 未注明 | The number of audio channels. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>mime_type</code> | <code>"audio/wav"</code> / <code>"audio/mp3"</code> / <code>"audio/aiff"</code> / <code>"audio/aac"</code> / <code>"audio/ogg"</code> / <code>"audio/flac"</code> / <code>"audio/mpeg"</code> / <code>"audio/m4a"</code> / <code>"audio/l16"</code> / <code>"audio/opus"</code> / <code>"audio/alaw"</code> / <code>"audio/mulaw"</code> / <code>"audio/webm"</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>rate</code> | <code>integer (int32)</code> | 可选；废弃 | 未注明 | Deprecated. Use sample_rate instead. The value is ignored. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>sample_rate</code> | <code>integer (int32)</code> | 可选 | 未注明 | The sample rate of the audio. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"audio"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>audio/wav</code>：WAV audio format
- <code>audio/mp3</code>：MP3 audio format
- <code>audio/aiff</code>：AIFF audio format
- <code>audio/aac</code>：AAC audio format
- <code>audio/ogg</code>：OGG audio format
- <code>audio/flac</code>：FLAC audio format
- <code>audio/mpeg</code>：MPEG audio format
- <code>audio/m4a</code>：M4A audio format
- <code>audio/l16</code>：L16 audio format
- <code>audio/opus</code>：OPUS audio format
- <code>audio/alaw</code>：ALAW audio format
- <code>audio/mulaw</code>：MULAW audio format
- <code>audio/webm</code>：WEBM audio format

## AudioResponseFormat {#schema-audioresponseformat}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for audio output format.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>bit_rate</code> | <code>integer (int32)</code> | 可选 | 未注明 | Bit rate in bits per second (bps). Only applicable for compressed formats (MP3, Opus). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>delivery</code> | <code>"inline"</code> / <code>"uri"</code> | 可选 | 未注明 | The delivery mode for the audio output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mime_type</code> | <code>"audio/mp3"</code> / <code>"audio/ogg_opus"</code> / <code>"audio/l16"</code> / <code>"audio/wav"</code> / <code>"audio/alaw"</code> / <code>"audio/mulaw"</code> | 可选 | 未注明 | The MIME type of the audio output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>sample_rate</code> | <code>integer (int32)</code> | 可选 | 未注明 | Sample rate in Hz. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"audio"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.delivery</code>）：

- <code>inline</code>：Audio data is returned inline in the response.
- <code>uri</code>：Audio data is returned as a URI.

枚举含义（<code>$.properties.mime_type</code>）：

- <code>audio/mp3</code>：MP3 audio format.
- <code>audio/ogg_opus</code>：OGG Opus audio format.
- <code>audio/l16</code>：Raw PCM (L16) audio format.
- <code>audio/wav</code>：WAV audio format.
- <code>audio/alaw</code>：A-law audio format.
- <code>audio/mulaw</code>：Mu-law audio format.

## CodeExecution {#schema-codeexecution}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model to execute code.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>type</code> | <code>"code_execution"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## CodeExecutionCallArguments {#schema-codeexecutioncallarguments}

出现位置：响应 / 错误 / SSE。

The arguments to pass to the code execution.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>code</code> | <code>string</code> | 可选 | 未注明 | The code to be executed. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>language</code> | <code>"python"</code> | 可选 | 未注明 | Programming language of the `code`. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.language</code>）：

- <code>python</code>：Python &gt;= 3.10, with numpy and simpy available.

## CodeExecutionCallDelta {#schema-codeexecutioncalldelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [CodeExecutionCallArguments](#schema-codeexecutioncallarguments) | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"code_execution_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## CodeExecutionCallStep {#schema-codeexecutioncallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Code execution call step.

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [CodeExecutionCallStepArguments](#schema-codeexecutioncallsteparguments) | 必填 | 未注明 | Required. The arguments to pass to the code execution. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"code_execution_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## CodeExecutionCallStepArguments {#schema-codeexecutioncallsteparguments}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The arguments to pass to the code execution.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>code</code> | <code>string</code> | 可选 | 未注明 | The code to be executed. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>language</code> | <code>"python"</code> | 可选 | 未注明 | Programming language of the `code`. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.language</code>）：

- <code>python</code>：Python &gt;= 3.10, with numpy and simpy available.

## CodeExecutionResultDelta {#schema-codeexecutionresultdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>result</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"code_execution_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## CodeExecutionResultStep {#schema-codeexecutionresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Code execution result step.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 | Whether the code execution resulted in an error. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>result</code> | <code>string</code> | 必填 | 未注明 | Required. The output of the code execution. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"code_execution_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## CodeMenderAgentConfig {#schema-codemenderagentconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for the CodeMender agent.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>find_request</code> | [FindRequest](#schema-findrequest) | 可选 | 未注明 | Parameters for finding vulnerabilities. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>fix_request</code> | [FixRequest](#schema-fixrequest) | 可选 | 未注明 | Parameters for fixing vulnerabilities. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>model</code> | <code>string</code> | 可选 | 未注明 | The name of the model to use for the CodeMender agent. One CodeMender session will only use one model. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>session_config</code> | [SessionConfig](#schema-sessionconfig) | 可选 | 未注明 | Optional session-specific configurations to override default agent behavior. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>session_id</code> | <code>string</code> | 可选 | 未注明 | Parameter for grouping multiple interactions that belong to the same CodeMender session. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"code-mender"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ComputerUse {#schema-computeruse}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model to interact with the computer.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>disabled_safety_policies</code> | Array&lt;<code>"financial_transactions"</code> / <code>"sensitive_data_modification"</code> / <code>"communication_tool"</code> / <code>"account_creation"</code> / <code>"data_modification"</code> / <code>"user_consent_management"</code> / <code>"legal_terms_and_agreements"</code>&gt; | 可选 | 未注明 | Optional. Disabled safety policies for computer use. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>enable_prompt_injection_detection</code> | <code>boolean</code> | 可选 | 未注明 | Whether enable the prompt injection detection check on computer-use request. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>environment</code> | <code>"browser"</code> / <code>"mobile"</code> / <code>"desktop"</code> | 可选 | 未注明 | The environment being operated. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>excluded_predefined_functions</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | The list of predefined functions that are excluded from the model call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"computer_use"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.disabled_safety_policies.items</code>）：

- <code>financial_transactions</code>：Safety policy for financial transactions.
- <code>sensitive_data_modification</code>：Safety policy for sensitive data modification.
- <code>communication_tool</code>：Safety policy for communication tools (e.g. Gmail, Chat, Meet).
- <code>account_creation</code>：Safety policy for account creation.
- <code>data_modification</code>：Safety policy for data modification.
- <code>user_consent_management</code>：Safety policy for user consent management.
- <code>legal_terms_and_agreements</code>：Safety policy for legal terms and agreements.

枚举含义（<code>$.properties.environment</code>）：

- <code>browser</code>：Operates in a web browser.
- <code>mobile</code>：Operates in a mobile environment.
- <code>desktop</code>：Operates in a desktop environment.

## Content {#schema-content}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The content of the response.

结构 / 允许值：[AudioContent](#schema-audiocontent) / [DocumentContent](#schema-documentcontent) / [ImageContent](#schema-imagecontent) / [TextContent](#schema-textcontent) / [VideoContent](#schema-videocontent)。

## CreateEnvironmentRequest {#schema-createenvironmentrequest}

出现位置：请求体（可能同时用于输出）。

Request for `CreateEnvironment`.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>from_environment</code> | <code>string</code> | 可选 | 未注明 | Optional. The source environment to copy/fork from. Format: `environments/&#123;environment_id&#125;` or `&#123;environment_id&#125;`. When specified, `sources` and `env` must be empty. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>network</code> | [EnvironmentNetworkEgressAllowlist](#schema-environmentnetworkegressallowlist) / <code>"disabled"</code> | 可选 | 未注明 | Network configuration for the environment. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>sources</code> | Array&lt;[Source](#schema-source)&gt; | 可选 | 未注明 | Sources to be mounted into the environment. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.network.oneOf.1</code>）：

- <code>disabled</code>：All network egress is blocked.

## CreateVoiceRequest {#schema-createvoicerequest}

出现位置：请求体（可能同时用于输出）。

Request message for `VoicesService.CreateVoice`.

结构 / 允许值：<code>object</code>。

官方 required：<code>voice</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>store</code> | <code>boolean</code> | 可选 | 未注明 | Optional. Whether the created voice is persisted and managed by Google.  * When `true`, Google stores the voice and returns `Voice.id` (for example,   `voice_abc123def456`), which can be managed via `GetVoice`, `ListVoices`,   and `DeleteVoice` and referenced by ID in synthesis requests. Stored   voices expire after 1 year of inactivity; using a stored voice in speech   synthesis or as a `base_voice` in `CreateVoice` extends its   `expire_time`. Projects are subject to a maximum active stored voice   quota; exceeding the quota returns `RESOURCE_EXHAUSTED`. * When `false` (default), the voice is not stored by Google and `Voice.key`   (for example, `voicekey_...`) is returned for client-side storage and   synthesis. Optional discovery metadata fields on `voice` are not   persisted or returned when `store` is `false`. * Required to be `true` when `voice.type` is `"prompted"`   (otherwise fails with `INVALID_ARGUMENT`). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>voice</code> | [Voice](#schema-voice) | 必填 | 未注明 | Required. The voice to create. `voice.model` is optional; if omitted, the service selects the default voice creation model. Output-only fields on `Voice` (`id`, `key`, `expire_time`, `usage`) are ignored if set. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## CreateWebhookRequest {#schema-createwebhookrequest}

出现位置：官方保留组件。

Request message for WebhookService.CreateWebhook.

结构 / 允许值：<code>object</code>。

官方 required：<code>webhook</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>webhook</code> | [Webhook](#schema-webhook) | 必填 | 未注明 | Required. The webhook to create. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## Credential {#schema-credential}

出现位置：响应 / 错误 / SSE。

Server-managed credential resource stored in Secret Manager.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>create_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The timestamp when the credential was created. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>id</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. Identifier. Unique identifier for the credential. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>status</code> | <code>"active"</code> / <code>"revoked"</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. Current status of the credential. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"bearer_token"</code> / <code>"oauth2"</code> / <code>"environment_variable"</code> | 可选；只读；官方描述：仅输出 | 未注明 | Required. Output only. The type of credential. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>update_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The timestamp when the credential was last updated. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.status</code>）：

- <code>active</code>：The credential is active and valid for use.
- <code>revoked</code>：The credential has been revoked and is no longer valid.

枚举含义（<code>$.properties.type</code>）：

- <code>bearer_token</code>：Static token injected as header. No refresh logic.
- <code>oauth2</code>：Auto-refresh expired access tokens using stored refresh token.
- <code>environment_variable</code>：Environment variable injected into sandbox container.

## CredentialCreateParams {#schema-credentialcreateparams}

出现位置：请求体（可能同时用于输出）。

Represents the fields of a Credential provided on creation.

结构 / 允许值：[EnvironmentVariableConfig](#schema-environmentvariableconfig) / [HttpBearerConfig](#schema-httpbearerconfig) / [OAuth2Config](#schema-oauth2config)。

## CredentialUpdateParams {#schema-credentialupdateparams}

出现位置：请求体（可能同时用于输出）。

Represents the fields of a Credential that can be updated.

结构 / 允许值：[EnvironmentVariableUpdateConfig](#schema-environmentvariableupdateconfig) / [HttpBearerUpdateConfig](#schema-httpbearerupdateconfig) / [OAuth2UpdateConfig](#schema-oauth2updateconfig)。

## DeepResearchAgentConfig {#schema-deepresearchagentconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for the Deep Research agent.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>collaborative_planning</code> | <code>boolean</code> | 可选 | 未注明 | Enables human-in-the-loop planning for the Deep Research agent. If set to true, the Deep Research agent will provide a research plan in its response. The agent will then proceed only if the user confirms the plan in the next turn. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>enable_bigquery_tool</code> | <code>boolean</code> | 可选 | 未注明 | Enables bigquery tool for the Deep Research agent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>thinking_summaries</code> | [ThinkingSummaries](#schema-thinkingsummaries) | 可选 | 未注明 | Whether to include thought summaries in the response. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"deep-research"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>visualization</code> | <code>"off"</code> / <code>"auto"</code> | 可选 | 未注明 | Whether to include visualizations in the response. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.visualization</code>）：

- <code>off</code>：Do not include visualizations.
- <code>auto</code>：Automatically include visualizations.

## DeleteVoiceResponse {#schema-deletevoiceresponse}

出现位置：响应 / 错误 / SSE。

Response message for `VoicesService.DeleteVoice`.

结构 / 允许值：<code>object</code>。

## DocumentContent {#schema-documentcontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A document content block.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 | The document content. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mime_type</code> | <code>"application/pdf"</code> / <code>"text/csv"</code> | 可选 | 未注明 | The mime type of the document. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"document"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 | The URI of the document. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>application/pdf</code>：PDF document format
- <code>text/csv</code>：CSV document format

## DocumentDelta {#schema-documentdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>mime_type</code> | <code>"application/pdf"</code> / <code>"text/csv"</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"document"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>application/pdf</code>：PDF document format
- <code>text/csv</code>：CSV document format

## DynamicAgentConfig {#schema-dynamicagentconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for dynamic agents.

结构 / 允许值：Map&lt;string, <code>any</code>&gt;。

官方 required：<code>type</code>。

自由属性：<code>true</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>type</code> | <code>"dynamic"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## EgressRule {#schema-egressrule}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A single domain allowlist rule with optional header injection.

结构 / 允许值：<code>object</code>。

官方 required：<code>domain</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>credential</code> | <code>string</code> | 可选 | 未注明 | Optional. Reference to a server-managed Credential resource by ID. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>domain</code> | <code>string</code> | 必填 | 未注明 | Domain to allow outbound requests to. Supports wildcards (e.g. '*.googleapis.com'). Use '*' to allow all domains. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>transform</code> | Array&lt;Map&lt;string, <code>string</code>&gt;&gt; / Map&lt;string, <code>string</code>&gt; | 可选 | 未注明 | Headers to inject on all outbound requests matching this domain. Accepts a single dict or a list of dicts. The egress proxy injects these automatically. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Empty {#schema-empty}

出现位置：响应 / 错误 / SSE。

A generic empty message that you can re-use to avoid defining duplicated empty messages in your APIs. A typical example is to use it as the request or the response type of an API method. For instance:      service Foo &#123;       rpc Bar(google.protobuf.Empty) returns (google.protobuf.Empty);     &#125;

结构 / 允许值：<code>object</code>。

## EnvVar {#schema-envvar}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

An environment variable to set in the execution environment.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>credential</code> | <code>string</code> | 可选 | 未注明 | Optional reference to a server-managed Credential resource by ID. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>value</code> | <code>string</code> | 可选 | 未注明 | Direct string value for plain environment variables. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Environment {#schema-environment}

出现位置：响应 / 错误 / SSE。

An execution environment for an agent.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>created</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time at which the environment was created in ISO 8601 format (YYYY-MM-DDThh:mm:ssZ). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>file_count</code> | <code>string (int64)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The number of files in the environment, output only. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>id</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. The ID of the environment. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>last_accessed</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time at which the environment was last accessed in ISO 8601 format (YYYY-MM-DDThh:mm:ssZ). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>network</code> | [EnvironmentNetworkEgressAllowlist](#schema-environmentnetworkegressallowlist) / <code>"disabled"</code> | 可选 | 未注明 | Network configuration for the environment. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>size_bytes</code> | <code>string (int64)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The total size of the environment files in bytes, output only. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>sources</code> | Array&lt;[Source](#schema-source)&gt; | 可选 | 未注明 | Sources to be mounted into the environment. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>status</code> | <code>"active"</code> / <code>"expired"</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The status of the environment container. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>updated</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time at which the environment was last updated in ISO 8601 format (YYYY-MM-DDThh:mm:ssZ). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.network.oneOf.1</code>）：

- <code>disabled</code>：All network egress is blocked.

枚举含义（<code>$.properties.status</code>）：

- <code>active</code>：官方未说明
- <code>expired</code>：官方未说明

## EnvironmentConfig {#schema-environmentconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for a custom environment.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>env</code> | Map&lt;string, [EnvVar](#schema-envvar)&gt; / <code>string</code> | 可选 | 未注明 | Environment variables to set in the sandbox environment. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>environment_id</code> | <code>string</code> | 可选 | 未注明 | Optional. The environment ID for the interaction. If specified, the request will update the existing environment instead of creating a new one. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>network</code> | [EnvironmentNetworkEgressAllowlist](#schema-environmentnetworkegressallowlist) / <code>"disabled"</code> | 可选 | 未注明 | Network configuration for the environment. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>sources</code> | Array&lt;[Source](#schema-source)&gt; | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"remote"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.network.oneOf.1</code>）：

- <code>disabled</code>：All network egress is blocked.

## EnvironmentFile {#schema-environmentfile}

出现位置：响应 / 错误 / SSE。

Metadata for a file or directory within an environment.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>created</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The creation time of the file/directory. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>mime_type</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The MIME type of the file (e.g., "text/python", "image/png"). Empty for directories. NOLINT | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>modified</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The modification time of the file/directory. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>name</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The name of the file or directory (e.g., "main.py" or "src"). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>path</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The full relative path within the environment (e.g., "workspace/src/main.py"). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>size_bytes</code> | <code>string (int64)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The size of the file/directory in bytes. NOLINT | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"file"</code> / <code>"directory"</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The type of the entry. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.type</code>）：

- <code>file</code>：A regular file.
- <code>directory</code>：A directory.

## EnvironmentNetworkEgressAllowlist {#schema-environmentnetworkegressallowlist}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Outbound networking configuration for the sandbox. Accepts an object with an 'allowlist' array to restrict traffic, or the string 'disabled' to turn off all network access. Omit entirely to allow all outbound traffic with no header injection.

结构 / 允许值：<code>object</code> / <code>"disabled"</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>$ (oneOf[0]).allowlist</code> | Array&lt;[EgressRule](#schema-egressrule)&gt; | 可选 | 未注明 | List of allowed outbound domains. Only requests to listed domains are permitted. Use [&#123;'domain': '*'&#125;] to allow all domains while still injecting headers on specific ones. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.oneOf.1</code>）：

- <code>disabled</code>：All network egress is blocked.

## EnvironmentVariableConfig {#schema-environmentvariableconfig}

出现位置：请求体（可能同时用于输出）。

Configuration for environment variable credentials.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>injection_location</code>、<code>type</code>、<code>value</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>id</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>injection_location</code> | [InjectionLocation](#schema-injectionlocation) / Array&lt;[InjectionLocation](#schema-injectionlocation)&gt; | 必填 | 未注明 | Required. Locations where the environment variable can be injected in outgoing HTTP requests. Must contain at least one location. Accepts either a single location (e.g. "header") or an array of locations. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>trusted_domains</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. List of domains allowed to receive this environment variable value in HTTP requests. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"environment_variable"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>value</code> | <code>string</code> | 必填；仅输入 | 未注明 | Required. Input only. Secret value of the environment variable. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## EnvironmentVariableUpdateConfig {#schema-environmentvariableupdateconfig}

出现位置：请求体（可能同时用于输出）。

Configuration for updating environment variable credentials.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>injection_location</code> | [InjectionLocation](#schema-injectionlocation) / Array&lt;[InjectionLocation](#schema-injectionlocation)&gt; | 可选 | 未注明 | Optional. Locations where the environment variable can be injected in outgoing HTTP requests. Accepts either a single location (e.g. "header") or an array of locations. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>trusted_domains</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. List of domains allowed to receive this environment variable value in HTTP requests. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"environment_variable"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>value</code> | <code>string</code> | 可选；仅输入 | 未注明 | Optional. Input only. Secret value of the environment variable. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Error {#schema-error}

出现位置：响应 / 错误 / SSE。

Error message from an interaction.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>code</code> | <code>string</code> | 可选 | 未注明 | A URI that identifies the error type. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>message</code> | <code>string</code> | 可选 | 未注明 | A human-readable error message. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ErrorEvent {#schema-errorevent}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>event_type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>error</code> | [Error](#schema-error) | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_id</code> | <code>string</code> | 可选 | 未注明 | The event_id token to be used to resume the interaction stream, from this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_type</code> | <code>"error"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ExaAISearchConfig {#schema-exaaisearchconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Used to specify configuration for ExaAISearch.

结构 / 允许值：<code>object</code>。

官方 required：<code>api_key</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>api_key</code> | <code>string</code> | 必填 | 未注明 | Required. The API key for ExaAiSearch. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>custom_config</code> | Map&lt;string, <code>any</code>&gt; | 可选 | 未注明 | Optional. This field can be used to pass any parameter from the Exa.ai Search API. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FileCitation {#schema-filecitation}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A file citation annotation.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>custom_metadata</code> | Map&lt;string, <code>any</code>&gt; | 可选 | 未注明 | User provided metadata about the retrieved context. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>document_uri</code> | <code>string</code> | 可选 | 未注明 | The URI of the file. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>end_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | End of the attributed segment, exclusive. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>file_name</code> | <code>string</code> | 可选 | 未注明 | The name of the file. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>media_id</code> | <code>string</code> | 可选 | 未注明 | Media ID in-case of image citations, if applicable. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>page_number</code> | <code>integer (int32)</code> | 可选 | 未注明 | Page number of the cited document, if applicable. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>source</code> | <code>string</code> | 可选 | 未注明 | Source attributed for a portion of the text. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>start_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | Start of segment of the response that is attributed to this source.  Index indicates the start of the segment, measured in bytes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"file_citation"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FileContent {#schema-filecontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Content of a single file in the codebase.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>content</code> | <code>string</code> | 可选 | 未注明 | The UTF-8 encoded text content of the file. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>path</code> | <code>string</code> | 可选 | 未注明 | The relative path of the file from the project root. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FileSearch {#schema-filesearch}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model to search files.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>file_search_store_names</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | The file search store names to search. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>metadata_filter</code> | <code>string</code> | 可选 | 未注明 | Metadata filter to apply to the semantic retrieval documents and chunks. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>top_k</code> | <code>integer (int32)</code> | 可选 | 未注明 | The number of semantic retrieval chunks to retrieve. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"file_search"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FileSearchCallDelta {#schema-filesearchcalldelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"file_search_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## FileSearchCallStep {#schema-filesearchcallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

File Search call step.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"file_search_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FileSearchResult {#schema-filesearchresult}

出现位置：响应 / 错误 / SSE。

The result of the File Search.

结构 / 允许值：<code>object</code>。

## FileSearchResultDelta {#schema-filesearchresultdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>result</code> | Array&lt;[FileSearchResult](#schema-filesearchresult)&gt; | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"file_search_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## FileSearchResultStep {#schema-filesearchresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

File Search result step.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"file_search_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Filter {#schema-filter}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Config for filters.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>metadata_filter</code> | <code>string</code> | 可选 | 未注明 | Optional. String for metadata filtering. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>vector_distance_threshold</code> | <code>number (double)</code> | 可选 | 未注明 | Optional. Only returns contexts with vector distance smaller than the threshold. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>vector_similarity_threshold</code> | <code>number (double)</code> | 可选 | 未注明 | Optional. Only returns contexts with vector similarity larger than the threshold. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FindRequest {#schema-findrequest}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Request parameters specific to FIND sessions, used for discovering vulnerabilities in a codebase.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>description</code> | <code>string</code> | 可选 | 未注明 | Additional context or custom instructions provided by the user to guide the vulnerability analysis. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>finding_id</code> | <code>string</code> | 可选 | 未注明 | The identifier of a specific finding to verify. This is primarily used in VERIFY mode to focus the agent's execution-based validation on a single vulnerability. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mode</code> | <code>"scan"</code> / <code>"verify"</code> | 可选 | 未注明 | The mode of the find session. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>source_files</code> | Array&lt;[FileContent](#schema-filecontent)&gt; | 可选 | 未注明 | A list of source files to provide as context for the scan. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mode</code>）：

- <code>scan</code>：Fast scan using only the initial classifier.
- <code>verify</code>：Performs classification followed by detailed investigation.

## FixRequest {#schema-fixrequest}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Request parameters specific to FIX sessions, used for generating and validating security patches.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>description</code> | <code>string</code> | 可选 | 未注明 | Additional context or custom instructions provided by the user to guide the patch generation process. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>finding_id</code> | <code>string</code> | 可选 | 未注明 | The identifier of the specific security finding to be remediated. This ID maps to a previously discovered vulnerability. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>source_files</code> | Array&lt;[FileContent](#schema-filecontent)&gt; | 可选 | 未注明 | A list of source files providing context for the remediation. These files are typically the ones containing the identified vulnerability. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Function {#schema-function}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>description</code> | <code>string</code> | 可选 | 未注明 | A description of the function. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | The name of the function. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>parameters</code> | <code>any</code> | 可选 | 未注明 | The JSON Schema for the function's parameters. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"function"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FunctionCallStep {#schema-functioncallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A function tool call step.

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>id</code>、<code>name</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | Map&lt;string, <code>any</code>&gt; | 必填 | 未注明 | Required. The arguments to pass to the function. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 必填 | 未注明 | Required. The name of the tool to call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"function_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FunctionResultDelta {#schema-functionresultdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>result</code> | Array&lt;[FunctionResultSubContent](#schema-functionresultsubcontent)&gt; / <code>object</code> / <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"function_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## FunctionResultStep {#schema-functionresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Result of a function tool call.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 | Whether the tool call resulted in an error. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | The name of the tool that was called. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>result</code> | Array&lt;[FunctionResultSubContent](#schema-functionresultsubcontent)&gt; / <code>object</code> / <code>string</code> | 必填 | 未注明 | Required. The result of the tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"function_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## FunctionResultSubContent {#schema-functionresultsubcontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：[ImageContent](#schema-imagecontent) / [TextContent](#schema-textcontent)。

## GenerationConfig {#schema-generationconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration parameters for model interactions.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>image_config</code> | [ImageConfig](#schema-imageconfig) | 可选；废弃 | 未注明 | Configuration for image interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>max_output_tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | The maximum number of tokens to include in the response. | 完整转发，由 Google 校验 | 映射：聊天 max_tokens |
| <code>seed</code> | <code>integer (int32)</code> | 可选 | 未注明 | Seed used in decoding for reproducibility. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>speech_config</code> | [SpeakerConfig](#schema-speakerconfig) / Array&lt;[SpeechConfig](#schema-speechconfig)&gt; | 可选 | 未注明 | Optional. Speech and multi-speaker configuration. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>stop_sequences</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | A list of character sequences that will stop output interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>temperature</code> | <code>number (float)</code> | 可选；废弃 | 未注明 | Controls the randomness of the output. | 完整转发，由 Google 校验 | 旧字段：同名聊天参数传入私有协议；新官方字段已废弃 |
| <code>thinking_level</code> | [ThinkingLevel](#schema-thinkinglevel) | 可选 | 未注明 | The level of thought tokens that the model should generate. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>thinking_summaries</code> | [ThinkingSummaries](#schema-thinkingsummaries) | 可选 | 未注明 | Whether to include thought summaries in the response. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>tool_choice</code> | [ToolChoiceConfig](#schema-toolchoiceconfig) / <code>"auto"</code> / <code>"any"</code> / <code>"none"</code> / <code>"validated"</code> | 可选 | 未注明 | The tool choice configuration. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>top_p</code> | <code>number (float)</code> | 可选；废弃 | 未注明 | The maximum cumulative probability of tokens to consider when sampling. | 完整转发，由 Google 校验 | 旧字段：同名聊天参数传入私有协议；新官方字段已废弃 |
| <code>transcription_config</code> | [TranscriptionConfig](#schema-transcriptionconfig) | 可选 | 未注明 | Optional. Configuration for speech recognition (transcription). If present, ASR is enabled. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>video_config</code> | [VideoConfig](#schema-videoconfig) | 可选 | 未注明 | Configuration for video generation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.tool_choice.oneOf.1</code>）：

- <code>auto</code>：Auto tool choice.
- <code>any</code>：Any tool choice.
- <code>none</code>：No tool choice.
- <code>validated</code>：Validated tool choice.

## GetEnvironmentFilesResponse {#schema-getenvironmentfilesresponse}

出现位置：响应 / 错误 / SSE。

Response for `GetEnvironmentFiles`.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>files</code> | Array&lt;[EnvironmentFile](#schema-environmentfile)&gt; | 可选 | 未注明 | If the requested path is a directory, this contains its contents. If the requested path is a file, this contains a single entry with the file's metadata. If alt=media was specified, this is empty (content is served via `blob`). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 | Pagination token for directory listing. NOLINT | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GetInteractionRequest {#schema-getinteractionrequest}

出现位置：官方保留组件。

Request for InteractionService.GetInteraction.

结构 / 允许值：<code>object</code>。

官方 required：<code>name</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>include_input</code> | <code>boolean</code> | 可选；废弃 | 未注明 | If true, includes the input in the response. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>last_event_id</code> | <code>string</code> | 可选 | 未注明 | If set, resumes the interaction stream from the chunk after the event marked by the event id. Can only be used if `stream` is true. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>name</code> | <code>string</code> | 必填 | 未注明 | Required. The name of the interaction to retrieve. Format: interactions/&#123;interaction&#125; | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>stream</code> | <code>boolean</code> | 可选 | 未注明 | If true, streams the interaction events as Server-Sent Events. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleMaps {#schema-googlemaps}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model to call Google Maps.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>enable_widget</code> | <code>boolean</code> | 可选 | 未注明 | Whether to return a widget context token in the tool call result of the response. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>latitude</code> | <code>number (double)</code> | 可选 | 未注明 | The latitude of the user's location. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>longitude</code> | <code>number (double)</code> | 可选 | 未注明 | The longitude of the user's location. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"google_maps"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleMapsCallArguments {#schema-googlemapscallarguments}

出现位置：响应 / 错误 / SSE。

The arguments to pass to the Google Maps tool.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>queries</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | The queries to be executed. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleMapsCallDelta {#schema-googlemapscalldelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [GoogleMapsCallArguments](#schema-googlemapscallarguments) | 可选 | 未注明 | The arguments to pass to the Google Maps tool. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"google_maps_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleMapsCallStep {#schema-googlemapscallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Google Maps call step.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [GoogleMapsCallStepArguments](#schema-googlemapscallsteparguments) | 可选 | 未注明 | The arguments to pass to the Google Maps tool. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"google_maps_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleMapsCallStepArguments {#schema-googlemapscallsteparguments}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The arguments to pass to the Google Maps tool.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>queries</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | The queries to be executed. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleMapsResult {#schema-googlemapsresult}

出现位置：响应 / 错误 / SSE。

The result of the Google Maps.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>places</code> | Array&lt;[Places](#schema-places)&gt; | 可选 | 未注明 | The places that were found. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>widget_context_token</code> | <code>string</code> | 可选 | 未注明 | Resource name of the Google Maps widget context token. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleMapsResultDelta {#schema-googlemapsresultdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>result</code> | Array&lt;[GoogleMapsResult](#schema-googlemapsresult)&gt; | 可选 | 未注明 | The results of the Google Maps. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"google_maps_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleMapsResultItem {#schema-googlemapsresultitem}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The result of the Google Maps.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>places</code> | Array&lt;[GoogleMapsResultPlaces](#schema-googlemapsresultplaces)&gt; | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>widget_context_token</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleMapsResultPlaces {#schema-googlemapsresultplaces}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>name</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>place_id</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>review_snippets</code> | Array&lt;[ReviewSnippet](#schema-reviewsnippet)&gt; | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleMapsResultStep {#schema-googlemapsresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Google Maps result step.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>result</code> | Array&lt;[GoogleMapsResultItem](#schema-googlemapsresultitem)&gt; | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"google_maps_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleSearch {#schema-googlesearch}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model to search Google.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>search_types</code> | Array&lt;<code>"web_search"</code> / <code>"image_search"</code> / <code>"enterprise_web_search"</code>&gt; | 可选 | 未注明 | The types of search grounding to enable. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"google_search"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.search_types.items</code>）：

- <code>web_search</code>：Setting this field enables web search. Only text results are returned.
- <code>image_search</code>：Setting this field enables image search. Image bytes are returned.
- <code>enterprise_web_search</code>：Setting this field enables enterprise web search.

## GoogleSearchCallArguments {#schema-googlesearchcallarguments}

出现位置：响应 / 错误 / SSE。

The arguments to pass to Google Search.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>queries</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Web search queries for the following-up web search. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleSearchCallDelta {#schema-googlesearchcalldelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [GoogleSearchCallArguments](#schema-googlesearchcallarguments) | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"google_search_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleSearchCallStep {#schema-googlesearchcallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Google Search call step.

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [GoogleSearchCallStepArguments](#schema-googlesearchcallsteparguments) | 必填 | 未注明 | Required. The arguments to pass to Google Search. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>search_type</code> | <code>"web_search"</code> / <code>"image_search"</code> / <code>"enterprise_web_search"</code> | 可选 | 未注明 | The type of search grounding enabled. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"google_search_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.search_type</code>）：

- <code>web_search</code>：Setting this field enables web search. Only text results are returned.
- <code>image_search</code>：Setting this field enables image search. Image bytes are returned.
- <code>enterprise_web_search</code>：Setting this field enables enterprise web search.

## GoogleSearchCallStepArguments {#schema-googlesearchcallsteparguments}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The arguments to pass to Google Search.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>queries</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Web search queries for the following-up web search. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleSearchResult {#schema-googlesearchresult}

出现位置：响应 / 错误 / SSE。

The result of the Google Search.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>search_suggestions</code> | <code>string</code> | 可选 | 未注明 | Web content snippet that can be embedded in a web page or an app webview. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleSearchResultDelta {#schema-googlesearchresultdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>result</code> | Array&lt;[GoogleSearchResult](#schema-googlesearchresult)&gt; | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"google_search_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## GoogleSearchResultItem {#schema-googlesearchresultitem}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The result of the Google Search.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>search_suggestions</code> | <code>string</code> | 可选 | 未注明 | Web content snippet that can be embedded in a web page or an app webview. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GoogleSearchResultStep {#schema-googlesearchresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Google Search result step.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 | Whether the Google Search resulted in an error. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>result</code> | Array&lt;[GoogleSearchResultItem](#schema-googlesearchresultitem)&gt; | 必填 | 未注明 | Required. The results of the Google Search. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"google_search_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## GroundingToolCount {#schema-groundingtoolcount}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The number of grounding tool counts.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>count</code> | <code>integer (int32)</code> | 可选 | 未注明 | The number of grounding tool counts. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"google_search"</code> / <code>"google_maps"</code> / <code>"retrieval"</code> | 可选 | 未注明 | The grounding tool type associated with the count. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.type</code>）：

- <code>google_search</code>：Grounding with Google Web Search and Image Search, &amp; Web Grounding for Enterprise.
- <code>google_maps</code>：Grounding with Google Maps.
- <code>retrieval</code>：Grounding with customer's data, for example, VertexAISearch.

## HarmCategory {#schema-harmcategory}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"hate_speech"</code> / <code>"dangerous_content"</code> / <code>"harassment"</code> / <code>"sexually_explicit"</code> / <code>"civic_integrity"</code> / <code>"image_hate"</code> / <code>"image_dangerous_content"</code> / <code>"image_harassment"</code> / <code>"image_sexually_explicit"</code> / <code>"jailbreak"</code>。

枚举含义（<code>$</code>）：

- <code>hate_speech</code>：Content that promotes violence or incites hatred against individuals or groups based on certain attributes.
- <code>dangerous_content</code>：Content that promotes, facilitates, or enables dangerous activities.
- <code>harassment</code>：Abusive, threatening, or content intended to bully, torment, or ridicule.
- <code>sexually_explicit</code>：Content that contains sexually explicit material.
- <code>civic_integrity</code>：Deprecated: Election filter is not longer supported. The harm category is civic integrity.
- <code>image_hate</code>：Images that contain hate speech.
- <code>image_dangerous_content</code>：Images that contain dangerous content.
- <code>image_harassment</code>：Images that contain harassment.
- <code>image_sexually_explicit</code>：Images that contain sexually explicit content.
- <code>jailbreak</code>：Prompts designed to bypass safety filters.

## HttpBearerConfig {#schema-httpbearerconfig}

出现位置：请求体（可能同时用于输出）。

Configuration for HTTP Bearer token credentials.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>token</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>header_name</code> | <code>string</code> | 可选 | 未注明 | Optional. Header name to inject the token into. Defaults to 'Authorization'. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>prefix</code> | <code>string</code> | 可选 | 未注明 | Optional. Prefix to prepend to the token. Defaults to 'Bearer'. Set to '' for no prefix. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>token</code> | <code>string</code> | 必填；仅输入 | 未注明 | Required. Input only. The static bearer token. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"bearer_token"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## HttpBearerUpdateConfig {#schema-httpbearerupdateconfig}

出现位置：请求体（可能同时用于输出）。

Configuration for updating HTTP Bearer token credentials.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>header_name</code> | <code>string</code> | 可选 | 未注明 | Optional. Header name to inject the token into. Defaults to 'Authorization'. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>prefix</code> | <code>string</code> | 可选 | 未注明 | Optional. Prefix to prepend to the token. Defaults to 'Bearer'. Set to '' for no prefix. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>token</code> | <code>string</code> | 可选；仅输入 | 未注明 | Optional. Input only. The static bearer token. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"bearer_token"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## HttpBody {#schema-httpbody}

出现位置：官方保留组件。

Message that represents an arbitrary HTTP body. It should only be used for payload formats that can't be represented as JSON, such as raw binary or an HTML page.   This message can be used both in streaming and non-streaming API methods in the request as well as the response.  It can be used as a top-level request field, which is convenient if one wants to extract parameters from either the URL or HTTP template into the request fields and also want access to the raw HTTP body.  Example:      message GetResourceRequest &#123;       // A unique request id.       string request_id = 1;        // The raw HTTP body is bound to this field.       google.api.HttpBody http_body = 2;      &#125;      service ResourceService &#123;       rpc GetResource(GetResourceRequest)         returns (google.api.HttpBody);       rpc UpdateResource(google.api.HttpBody)         returns (google.protobuf.Empty);      &#125;  Example with streaming methods:      service CaldavService &#123;       rpc GetCalendar(stream google.api.HttpBody)         returns (stream google.api.HttpBody);       rpc UpdateCalendar(stream google.api.HttpBody)         returns (stream google.api.HttpBody);      &#125;  Use of this type only changes how the request and response bodies are handled, all other features will continue to work unchanged.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>content_type</code> | <code>string</code> | 可选 | 未注明 | The HTTP Content-Type header value specifying the content type of the body. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 | The HTTP request/response body as raw binary. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>extensions</code> | Array&lt;Map&lt;string, <code>any</code>&gt;&gt; | 可选 | 未注明 | Application specific response metadata. Must be set in the first response for streaming APIs. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## HybridSearch {#schema-hybridsearch}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Config for Hybrid Search.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>alpha</code> | <code>number (float)</code> | 可选 | 未注明 | Optional. Alpha value controls the weight between dense and sparse vector search results. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ImageConfig {#schema-imageconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The configuration for image interaction.

结构 / 允许值：<code>object</code>。

此 schema 已废弃。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>aspect_ratio</code> | <code>"1:1"</code> / <code>"2:3"</code> / <code>"3:2"</code> / <code>"3:4"</code> / <code>"4:3"</code> / <code>"4:5"</code> / <code>"5:4"</code> / <code>"9:16"</code> / <code>"16:9"</code> / <code>"21:9"</code> / <code>"1:8"</code> / <code>"8:1"</code> / <code>"1:4"</code> / <code>"4:1"</code> | 可选 | 未注明 | The aspect ratio of the image to generate. Supported aspect ratios: 1:1, 2:3, 3:2, 3:4, 4:3, 9:16, 16:9, 21:9.  If not specified, the model will choose a default aspect ratio based on any reference images provided. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>image_size</code> | <code>"1K"</code> / <code>"2K"</code> / <code>"4K"</code> / <code>"512"</code> | 可选 | 未注明 | Specifies the size of generated images. Supported values are `1K`, `2K`, `4K`. If not specified, the model will use default value `1K`. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.aspect_ratio</code>）：

- <code>1:1</code>：1:1 aspect ratio.
- <code>2:3</code>：2:3 aspect ratio.
- <code>3:2</code>：3:2 aspect ratio.
- <code>3:4</code>：3:4 aspect ratio.
- <code>4:3</code>：4:3 aspect ratio.
- <code>4:5</code>：4:5 aspect ratio.
- <code>5:4</code>：5:4 aspect ratio.
- <code>9:16</code>：9:16 aspect ratio.
- <code>16:9</code>：16:9 aspect ratio.
- <code>21:9</code>：21:9 aspect ratio.
- <code>1:8</code>：1:8 aspect ratio.
- <code>8:1</code>：8:1 aspect ratio.
- <code>1:4</code>：1:4 aspect ratio.
- <code>4:1</code>：4:1 aspect ratio.

枚举含义（<code>$.properties.image_size</code>）：

- <code>1K</code>：1K image size.
- <code>2K</code>：2K image size.
- <code>4K</code>：4K image size.
- <code>512</code>：512 image size.

## ImageContent {#schema-imagecontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

An image content block.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 | The image content. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mime_type</code> | <code>"image/png"</code> / <code>"image/jpeg"</code> / <code>"image/webp"</code> / <code>"image/heic"</code> / <code>"image/heif"</code> / <code>"image/gif"</code> / <code>"image/bmp"</code> / <code>"image/tiff"</code> | 可选 | 未注明 | The mime type of the image. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>resolution</code> | [MediaResolution](#schema-mediaresolution) | 可选 | 未注明 | The resolution of the media. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"image"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 | The URI of the image. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>image/png</code>：PNG image format
- <code>image/jpeg</code>：JPEG image format
- <code>image/webp</code>：WebP image format
- <code>image/heic</code>：HEIC image format
- <code>image/heif</code>：HEIF image format
- <code>image/gif</code>：GIF image format
- <code>image/bmp</code>：BMP image format
- <code>image/tiff</code>：TIFF image format

## ImageDelta {#schema-imagedelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>mime_type</code> | <code>"image/png"</code> / <code>"image/jpeg"</code> / <code>"image/webp"</code> / <code>"image/heic"</code> / <code>"image/heif"</code> / <code>"image/gif"</code> / <code>"image/bmp"</code> / <code>"image/tiff"</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>resolution</code> | [MediaResolution](#schema-mediaresolution) | 可选 | 未注明 | The resolution of the media. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"image"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>image/png</code>：PNG image format
- <code>image/jpeg</code>：JPEG image format
- <code>image/webp</code>：WebP image format
- <code>image/heic</code>：HEIC image format
- <code>image/heif</code>：HEIF image format
- <code>image/gif</code>：GIF image format
- <code>image/bmp</code>：BMP image format
- <code>image/tiff</code>：TIFF image format

## ImageResponseFormat {#schema-imageresponseformat}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for image output format.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>aspect_ratio</code> | <code>"1:1"</code> / <code>"2:3"</code> / <code>"3:2"</code> / <code>"3:4"</code> / <code>"4:3"</code> / <code>"4:5"</code> / <code>"5:4"</code> / <code>"9:16"</code> / <code>"16:9"</code> / <code>"21:9"</code> / <code>"1:8"</code> / <code>"8:1"</code> / <code>"1:4"</code> / <code>"4:1"</code> | 可选 | 未注明 | The aspect ratio for the image output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>delivery</code> | <code>"inline"</code> / <code>"uri"</code> | 可选 | 未注明 | The delivery mode for the image output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>image_size</code> | <code>"512"</code> / <code>"1K"</code> / <code>"2K"</code> / <code>"4K"</code> | 可选 | 未注明 | The size of the image output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mime_type</code> | <code>"image/jpeg"</code> | 可选 | 未注明 | The MIME type of the image output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"image"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.aspect_ratio</code>）：

- <code>1:1</code>：1:1 aspect ratio.
- <code>2:3</code>：2:3 aspect ratio.
- <code>3:2</code>：3:2 aspect ratio.
- <code>3:4</code>：3:4 aspect ratio.
- <code>4:3</code>：4:3 aspect ratio.
- <code>4:5</code>：4:5 aspect ratio.
- <code>5:4</code>：5:4 aspect ratio.
- <code>9:16</code>：9:16 aspect ratio.
- <code>16:9</code>：16:9 aspect ratio.
- <code>21:9</code>：21:9 aspect ratio.
- <code>1:8</code>：1:8 aspect ratio.
- <code>8:1</code>：8:1 aspect ratio.
- <code>1:4</code>：1:4 aspect ratio.
- <code>4:1</code>：4:1 aspect ratio.

枚举含义（<code>$.properties.delivery</code>）：

- <code>inline</code>：Image data is returned inline in the response.
- <code>uri</code>：Image data is returned as a URI.

枚举含义（<code>$.properties.image_size</code>）：

- <code>512</code>：512px image size.
- <code>1K</code>：1K image size.
- <code>2K</code>：2K image size.
- <code>4K</code>：4K image size.

枚举含义（<code>$.properties.mime_type</code>）：

- <code>image/jpeg</code>：JPEG image format.

## InjectionLocation {#schema-injectionlocation}

出现位置：请求体（可能同时用于输出）。

结构 / 允许值：<code>"header"</code> / <code>"query"</code> / <code>"body"</code>。

枚举含义（<code>$</code>）：

- <code>header</code>：Injected into HTTP request headers.
- <code>query</code>：Injected into HTTP URL query parameters.
- <code>body</code>：Injected into HTTP request body.

## Interaction {#schema-interaction}

出现位置：响应 / 错误 / SSE。

The Interaction resource.

结构 / 允许值：<code>object</code>。

官方 required：<code>status</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>agent</code> | [AgentOption](#schema-agentoption) | 可选 | 未注明 | The name of the `Agent` used for generating the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>agent_config</code> | [AntigravityAgentConfig](#schema-antigravityagentconfig) / [CodeMenderAgentConfig](#schema-codemenderagentconfig) / [DeepResearchAgentConfig](#schema-deepresearchagentconfig) / [DynamicAgentConfig](#schema-dynamicagentconfig) | 可选 | 未注明 | Configuration parameters for the agent interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>background</code> | <code>boolean</code> | 可选；仅输入 | 未注明 | Input only. Whether to run the model interaction in the background. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>cached_content</code> | <code>string</code> | 可选；废弃 | 未注明 | The name of the cached content used as context to serve the prediction. Note: only used in explicit caching, where users can have control over caching (e.g. what content to cache) and enjoy guaranteed cost savings. Format: cachedContents/&#123;cachedContent&#125; | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>continuation_token</code> | <code>string (byte)</code> | 可选 | 未注明 | Opaque token to resume a long decode. Output: set when status is INCOMPLETE and decoding can be resumed. Input: pass the latest token back unchanged in CreateInteraction to continue decoding. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>created</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Required. Output only. The time at which the response was created in ISO 8601 format (YYYY-MM-DDThh:mm:ssZ). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>environment</code> | [EnvironmentConfig](#schema-environmentconfig) / <code>string</code> | 可选 | 未注明 | The environment configuration for the interaction. Can be an object specifying remote environment sources or a string referencing an existing environment ID. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>environment_id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The environment ID for the interaction. Only populated if environment config is set in the request. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>errors</code> | Array&lt;[Error](#schema-error)&gt; | 可选；只读；官方描述：仅输出 | 未注明 | Output only. Diagnostic faults / platform errors recorded on the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>generation_config</code> | [GenerationConfig](#schema-generationconfig) | 可选；仅输入 | 未注明 | Input only. Configuration parameters for the model interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | <code>default=""</code> | Required. Output only. A unique identifier for the interaction completion. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>input</code> | [InteractionsInput](#schema-interactionsinput) | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>labels</code> | Map&lt;string, <code>string</code>&gt; | 可选 | 未注明 | The labels with user-defined metadata for the request.  Label keys and values can be no longer than 63 characters (Unicode codepoints) and can only contain lowercase letters, numeric characters, underscores, and dashes. International characters are allowed. Label values are optional. Label keys must start with a letter. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>model</code> | [ModelOption](#schema-modeloption) | 可选 | 未注明 | The name of the `Model` used for generating the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>previous_interaction_id</code> | <code>string</code> | 可选 | 未注明 | The ID of the previous interaction, if any. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>response_format</code> | [ResponseFormat](#schema-responseformat) / Array&lt;[ResponseFormat](#schema-responseformat)&gt; | 可选 | 未注明 | Enforces that the generated response is a JSON object that complies with the JSON schema specified in this field. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>response_mime_type</code> | <code>string</code> | 可选；废弃 | 未注明 | The mime type of the response. This is required if response_format is set. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>response_modalities</code> | Array&lt;[ResponseModality](#schema-responsemodality)&gt; | 可选；废弃 | 未注明 | The requested modalities of the response (TEXT, IMAGE, AUDIO). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>safety_settings</code> | Array&lt;[SafetySetting](#schema-safetysetting)&gt; | 可选 | 未注明 | Safety settings for the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>service_tier</code> | [ServiceTier](#schema-servicetier) | 可选 | 未注明 | The service tier for the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>status</code> | <code>"in_progress"</code> / <code>"requires_action"</code> / <code>"completed"</code> / <code>"failed"</code> / <code>"cancelled"</code> / <code>"incomplete"</code> / <code>"budget_exceeded"</code> / <code>"queued"</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. The status of the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>steps</code> | Array&lt;[Step](#schema-step)&gt; | 可选；只读；官方描述：仅输出 | 未注明 | Required. Output only. The steps that make up the interaction, when included in the response. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>store</code> | <code>boolean</code> | 可选；仅输入 | 未注明 | Input only. Whether to store the response and request for later retrieval. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>stream</code> | <code>boolean</code> | 可选；仅输入 | 未注明 | Input only. Whether the interaction will be streamed. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>system_instruction</code> | <code>string</code> | 可选 | 未注明 | System instruction for the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>tools</code> | Array&lt;[Tool](#schema-tool)&gt; | 可选 | 未注明 | A list of tool declarations the model may call during interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>updated</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Required. Output only. The time at which the response was last updated in ISO 8601 format (YYYY-MM-DDThh:mm:ssZ). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>usage</code> | [Usage](#schema-usage) | 可选；只读；官方描述：仅输出 | 未注明 | Output only. Statistics on the interaction request's token usage. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>webhook_config</code> | [WebhookConfig](#schema-webhookconfig) | 可选 | 未注明 | Optional. Webhook configuration for receiving notifications when the interaction completes. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.status</code>）：

- <code>in_progress</code>：The interaction is in progress.
- <code>requires_action</code>：The interaction requires action/input from the user.
- <code>completed</code>：The interaction is completed.
- <code>failed</code>：The interaction failed.
- <code>cancelled</code>：The interaction was cancelled.
- <code>incomplete</code>：The interaction is completed, but contains incomplete results (e.g. hitting max_tokens).
- <code>budget_exceeded</code>：Deprecated: Token and execution budget exhaustion returns INCOMPLETE (11).
- <code>queued</code>：The interaction is queued, waiting for processing (e.g. waiting for off-peak capacity).

## InteractionCompletedEvent {#schema-interactioncompletedevent}

出现位置：响应 / 错误 / SSE。

Signals that the Interaction completed. Sent when the Interaction receives Complete/Cancel or naturally terminates. No more input can be sent to the Interaction after this.

结构 / 允许值：<code>object</code>。

官方 required：<code>event_type</code>、<code>interaction</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>event_id</code> | <code>string</code> | 可选 | 未注明 | The event_id token to be used to resume the interaction stream, from this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_type</code> | <code>"interaction.completed"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>interaction</code> | [InteractionSseEventInteraction](#schema-interactionsseeventinteraction) | 必填 | 未注明 | Required. Partial completed interaction resource emitted at the end of the stream. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## InteractionCreatedEvent {#schema-interactioncreatedevent}

出现位置：响应 / 错误 / SSE。

Server response confirming that a new interaction was created.

结构 / 允许值：<code>object</code>。

官方 required：<code>event_type</code>、<code>interaction</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>event_id</code> | <code>string</code> | 可选 | 未注明 | The event_id token to be used to resume the interaction stream, from this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_type</code> | <code>"interaction.created"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>interaction</code> | [InteractionSseEventInteraction](#schema-interactionsseeventinteraction) | 必填 | 未注明 | Required. Partial interaction resource emitted when the stream is created. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## InteractionSseEventInteraction {#schema-interactionsseeventinteraction}

出现位置：响应 / 错误 / SSE。

Partial interaction resource emitted by interaction lifecycle SSE events. Streaming lifecycle payloads may omit fields that are only available on full non-streaming Interaction responses.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>status</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>agent</code> | <code>string</code> | 可选 | 未注明 | The agent to interact with. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>continuation_token</code> | <code>string (byte)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. Opaque token to resume a long decode when status is incomplete. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>created</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time at which the response was created in ISO 8601 format. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>id</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. A unique identifier for the interaction completion. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>model</code> | <code>string</code> | 可选 | 未注明 | The model that will complete your prompt. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>object</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The resource type. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>service_tier</code> | [ServiceTier](#schema-servicetier) | 可选 | 未注明 | The service tier for the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>status</code> | <code>"in_progress"</code> / <code>"requires_action"</code> / <code>"completed"</code> / <code>"failed"</code> / <code>"cancelled"</code> / <code>"incomplete"</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. The status of the interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>steps</code> | Array&lt;[Step](#schema-step)&gt; | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The steps that make up the interaction, if included in this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>updated</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time at which the response was last updated in ISO 8601 format. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>usage</code> | [Usage](#schema-usage) | 可选；只读；官方描述：仅输出 | 未注明 | Output only. Statistics on the interaction request's token usage. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.status</code>）：

- <code>in_progress</code>：The interaction is in progress.
- <code>requires_action</code>：The interaction requires action/input from the user.
- <code>completed</code>：The interaction is completed.
- <code>failed</code>：The interaction failed.
- <code>cancelled</code>：The interaction was cancelled.
- <code>incomplete</code>：The interaction is completed, but contains incomplete results (e.g. hitting max_tokens).

## InteractionSseStreamEnvelope {#schema-interactionssestreamenvelope}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>data</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | [InteractionSseStreamEvent](#schema-interactionssestreamevent) | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## InteractionSseStreamEvent {#schema-interactionssestreamevent}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：[ErrorEvent](#schema-errorevent) / [InteractionCompletedEvent](#schema-interactioncompletedevent) / [InteractionCreatedEvent](#schema-interactioncreatedevent) / [InteractionStatusUpdate](#schema-interactionstatusupdate) / [StepDelta](#schema-stepdelta) / [StepStart](#schema-stepstart) / [StepStop](#schema-stepstop)。

## InteractionStatusUpdate {#schema-interactionstatusupdate}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>event_type</code>、<code>interaction_id</code>、<code>status</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>event_id</code> | <code>string</code> | 可选 | 未注明 | The event_id token to be used to resume the interaction stream, from this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_type</code> | <code>"interaction.status_update"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>interaction_id</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>status</code> | <code>"in_progress"</code> / <code>"requires_action"</code> / <code>"completed"</code> / <code>"failed"</code> / <code>"cancelled"</code> / <code>"incomplete"</code> / <code>"budget_exceeded"</code> / <code>"queued"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.status</code>）：

- <code>in_progress</code>：The interaction is in progress.
- <code>requires_action</code>：The interaction requires action/input from the user.
- <code>completed</code>：The interaction is completed.
- <code>failed</code>：The interaction failed.
- <code>cancelled</code>：The interaction was cancelled.
- <code>incomplete</code>：The interaction is completed, but contains incomplete results (e.g. hitting max_tokens).
- <code>budget_exceeded</code>：Deprecated: Token and execution budget exhaustion returns INCOMPLETE (11).
- <code>queued</code>：The interaction is queued, waiting for processing (e.g. waiting for off-peak capacity).

## InteractionsInput {#schema-interactionsinput}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The input for the interaction.

结构 / 允许值：[Content](#schema-content) / Array&lt;[Step](#schema-step)&gt; / Array&lt;[Content](#schema-content)&gt; / <code>string</code>。

## ListAgentsResponse {#schema-listagentsresponse}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>agents</code> | Array&lt;[Agent](#schema-agent)&gt; | 可选 | 未注明 | The list of agents. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 | A token to retrieve the next page of results. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ListCredentialsResponse {#schema-listcredentialsresponse}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>credentials</code> | Array&lt;[Credential](#schema-credential)&gt; | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ListEnvironmentsResponse {#schema-listenvironmentsresponse}

出现位置：响应 / 错误 / SSE。

Response for `ListEnvironments`.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>environments</code> | Array&lt;[Environment](#schema-environment)&gt; | 可选 | 未注明 | Environments belonging to the provided project. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 | Pagination token. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ListTriggerExecutionsResponse {#schema-listtriggerexecutionsresponse}

出现位置：响应 / 错误 / SSE。

Response message for TriggerService.ListTriggerExecutions.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 | A page token, received from a previous `ListTriggerExecutions` call. Provide this to retrieve the subsequent page. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>trigger_executions</code> | Array&lt;[TriggerExecution](#schema-triggerexecution)&gt; | 可选 | 未注明 | The list of trigger executions. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ListTriggersResponse {#schema-listtriggersresponse}

出现位置：响应 / 错误 / SSE。

Response message for TriggerService.ListTriggers.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 | A page token, received from a previous `ListTriggers` call. Provide this to retrieve the subsequent page. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>triggers</code> | Array&lt;[Trigger](#schema-trigger)&gt; | 可选 | 未注明 | The list of triggers. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ListVoicesResponse {#schema-listvoicesresponse}

出现位置：响应 / 错误 / SSE。

Response message for `VoicesService.ListVoices`.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 | A token that can be sent as `page_token` to retrieve the next page. If empty, there are no subsequent pages. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>voices</code> | Array&lt;[Voice](#schema-voice)&gt; | 可选 | 未注明 | The voices from the specified collection. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ListWebhooksResponse {#schema-listwebhooksresponse}

出现位置：响应 / 错误 / SSE。

Response message for WebhookService.ListWebhooks.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>next_page_token</code> | <code>string</code> | 可选 | 未注明 | A token, which can be sent as `page_token` to retrieve the next page. If this field is omitted, there are no subsequent pages. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>webhooks</code> | Array&lt;[Webhook](#schema-webhook)&gt; | 可选 | 未注明 | The webhooks. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## McpServer {#schema-mcpserver}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A MCPServer is a server that can be called by the model to perform actions.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>allowed_tools</code> | Array&lt;[AllowedTools](#schema-allowedtools)&gt; | 可选 | 未注明 | The allowed tools. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>headers</code> | Map&lt;string, <code>string</code>&gt; | 可选 | 未注明 | Optional: Fields for authentication headers, timeouts, etc., if needed. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | The name of the MCPServer. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"mcp_server"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 | The full URL for the MCPServer endpoint. Example: "https://api.example.com/mcp" | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## McpServerToolCallDelta {#schema-mcpservertoolcalldelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>name</code>、<code>server_name</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | Map&lt;string, <code>any</code>&gt; | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>name</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>server_name</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"mcp_server_tool_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## McpServerToolCallStep {#schema-mcpservertoolcallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

MCPServer tool call step.

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>id</code>、<code>name</code>、<code>server_name</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | Map&lt;string, <code>any</code>&gt; | 必填 | 未注明 | Required. The JSON object of arguments for the function. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 必填 | 未注明 | Required. The name of the tool which was called. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>server_name</code> | <code>string</code> | 必填 | 未注明 | Required. The name of the used MCP server. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"mcp_server_tool_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## McpServerToolResultDelta {#schema-mcpservertoolresultdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>name</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>result</code> | Array&lt;[FunctionResultSubContent](#schema-functionresultsubcontent)&gt; / <code>object</code> / <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>server_name</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"mcp_server_tool_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## McpServerToolResultStep {#schema-mcpservertoolresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

MCPServer tool result step.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | Name of the tool which is called for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>result</code> | Array&lt;[FunctionResultSubContent](#schema-functionresultsubcontent)&gt; / <code>object</code> / <code>string</code> | 必填 | 未注明 | Required. The output from the MCP server call. Can be simple text or rich content. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>server_name</code> | <code>string</code> | 可选 | 未注明 | The name of the used MCP server. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"mcp_server_tool_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## MediaProcessing {#schema-mediaprocessing}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：[StaticMediaProcessing](#schema-staticmediaprocessing)。

## MediaResolution {#schema-mediaresolution}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"low"</code> / <code>"medium"</code> / <code>"high"</code> / <code>"ultra_high"</code>。

枚举含义（<code>$</code>）：

- <code>low</code>：Low resolution.
- <code>medium</code>：Medium resolution.
- <code>high</code>：High resolution.
- <code>ultra_high</code>：Ultra high resolution.

## ModalityTokens {#schema-modalitytokens}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The token count for a single response modality.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>modality</code> | [ResponseModality](#schema-responsemodality) | 可选 | 未注明 | The modality associated with the token count. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | Number of tokens for the modality. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ModelInteraction {#schema-modelinteraction}

出现位置：请求体（可能同时用于输出）。

Interaction for generating the completion using models.

结构 / 允许值：<code>object</code>。

官方 required：<code>created</code>、<code>id</code>、<code>model</code>、<code>status</code>、<code>steps</code>、<code>updated</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>background</code> | <code>boolean</code> | 可选；仅输入 | 未注明 | Input only. Whether to run the model interaction in the background. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>cached_content</code> | <code>string</code> | 可选；废弃 | 未注明 | The name of the cached content used as context to serve the prediction. Note: only used in explicit caching, where users can have control over caching (e.g. what content to cache) and enjoy guaranteed cost savings. Format: cachedContents/&#123;cachedContent&#125; | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>continuation_token</code> | <code>string (byte)</code> | 可选 | 未注明 | Opaque token to resume a long decode. Output: set when status is INCOMPLETE and decoding can be resumed. Input: pass the latest token back unchanged in CreateInteraction to continue decoding. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>created</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. The time at which the response was created in ISO 8601 format (YYYY-MM-DDThh:mm:ssZ). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>environment</code> | [EnvironmentConfig](#schema-environmentconfig) / <code>string</code> | 可选 | 未注明 | The environment configuration for the interaction. Can be an object specifying remote environment sources or a string referencing an existing environment ID. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>environment_id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The environment ID for the interaction. Only populated if environment config is set in the request. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>generation_config</code> | [GenerationConfig](#schema-generationconfig) | 可选；仅输入 | 未注明 | Input only. Configuration parameters for the model interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. A unique identifier for the interaction completion. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>input</code> | [InteractionsInput](#schema-interactionsinput) | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 部分：messages 仅文本；没有 Content/Step JSON 接口 |
| <code>labels</code> | Map&lt;string, <code>string</code>&gt; | 可选 | 未注明 | The labels with user-defined metadata for the request.  Label keys and values can be no longer than 63 characters (Unicode codepoints) and can only contain lowercase letters, numeric characters, underscores, and dashes. International characters are allowed. Label values are optional. Label keys must start with a letter. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>model</code> | [ModelOption](#schema-modeloption) | 必填 | 未注明 | The name of the `Model` used for generating the interaction. | 完整转发，由 Google 校验 | 映射：聊天 model（实际模型目录决定） |
| <code>previous_interaction_id</code> | <code>string</code> | 可选 | 未注明 | The ID of the previous interaction, if any. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>response_format</code> | [ResponseFormat](#schema-responseformat) / Array&lt;[ResponseFormat](#schema-responseformat)&gt; | 可选 | 未注明 | Enforces that the generated response is a JSON object that complies with the JSON schema specified in this field. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>response_mime_type</code> | <code>string</code> | 可选；废弃 | 未注明 | The mime type of the response. This is required if response_format is set. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>response_modalities</code> | Array&lt;[ResponseModality](#schema-responsemodality)&gt; | 可选；废弃 | 未注明 | The requested modalities of the response (TEXT, IMAGE, AUDIO). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>safety_settings</code> | Array&lt;[SafetySetting](#schema-safetysetting)&gt; | 可选 | 未注明 | Safety settings for the interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>service_tier</code> | [ServiceTier](#schema-servicetier) | 可选 | 未注明 | The service tier for the interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>status</code> | <code>"in_progress"</code> / <code>"requires_action"</code> / <code>"completed"</code> / <code>"failed"</code> / <code>"cancelled"</code> / <code>"incomplete"</code> / <code>"budget_exceeded"</code> / <code>"queued"</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. The status of the interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>store</code> | <code>boolean</code> | 可选；仅输入 | 未注明 | Input only. Whether to store the response and request for later retrieval. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>stream</code> | <code>boolean</code> | 可选；仅输入 | 未注明 | Input only. Whether the interaction will be streamed. | 完整转发，由 Google 校验 | 部分：聊天 SSE；不是 Interactions 事件流 |
| <code>system_instruction</code> | <code>string</code> | 可选 | 未注明 | System instruction for the interaction. | 完整转发，由 Google 校验 | 映射：messages 中的 system 文本 |
| <code>tools</code> | Array&lt;[Tool](#schema-tool)&gt; | 可选 | 未注明 | A list of tool declarations the model may call during interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>updated</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. The time at which the response was last updated in ISO 8601 format (YYYY-MM-DDThh:mm:ssZ). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>webhook_config</code> | [WebhookConfig](#schema-webhookconfig) | 可选 | 未注明 | Optional. Webhook configuration for receiving notifications when the interaction completes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.status</code>）：

- <code>in_progress</code>：The interaction is in progress.
- <code>requires_action</code>：The interaction requires action/input from the user.
- <code>completed</code>：The interaction is completed.
- <code>failed</code>：The interaction failed.
- <code>cancelled</code>：The interaction was cancelled.
- <code>incomplete</code>：The interaction is completed, but contains incomplete results (e.g. hitting max_tokens).
- <code>budget_exceeded</code>：Deprecated: Token and execution budget exhaustion returns INCOMPLETE (11).
- <code>queued</code>：The interaction is queued, waiting for processing (e.g. waiting for off-peak capacity).

## ModelOption {#schema-modeloption}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The model that will complete your prompt.\n\nSee [models](https://ai.google.dev/gemini-api/docs/models) for additional details.

结构 / 允许值：<code>"gemini-2.5-flash"</code> / <code>"gemini-2.5-pro"</code> / <code>"gemma-4-26b-a4b-it"</code> / <code>"gemma-4-31b-it"</code> / <code>"gemini-flash-latest"</code> / <code>"gemini-flash-lite-latest"</code> / <code>"gemini-pro-latest"</code> / <code>"gemini-2.5-flash-lite"</code> / <code>"gemini-2.5-flash-image"</code> / <code>"gemini-3-flash-preview"</code> / <code>"gemini-3.1-pro-preview"</code> / <code>"gemini-3.1-pro-preview-customtools"</code> / <code>"gemini-3.1-flash-lite"</code> / <code>"gemini-3-pro-image"</code> / <code>"nano-banana-pro-preview"</code> / <code>"gemini-3.1-flash-image"</code> / <code>"gemini-3.1-flash-tts-preview"</code> / <code>"gemini-3.5-flash"</code> / <code>"gemini-3.6-flash"</code> / <code>"gemini-3.7-flash"</code> / <code>"gemini-3.8-flash"</code> / <code>"gemini-3.8-flash-tts"</code> / <code>"gemini-3.8-flash-lite-tts"</code> / <code>"lyria-3-clip-preview"</code> / <code>"lyria-3-pro-preview"</code> / <code>"gemini-robotics-er-1.6-preview"</code> / <code>"gemini-robotics-er-2-preview"</code> / <code>"lyria-3.5"</code> / <code>"gemini-omni-1.1-flash"</code> / <code>"gemini-omni-flash-preview"</code>。

## ModelOutputStep {#schema-modeloutputstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Output generated by the model.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>content</code> | Array&lt;[Content](#schema-content)&gt; | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>error</code> | [Status](#schema-status) | 可选；废弃 | 未注明 | The error result of the operation in case of failure or cancellation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"model_output"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## OAuth2Config {#schema-oauth2config}

出现位置：请求体（可能同时用于输出）。

Configuration for OAuth2 credentials with automatic token refresh.

结构 / 允许值：<code>object</code>。

官方 required：<code>client_id</code>、<code>client_secret</code>、<code>id</code>、<code>refresh_token</code>、<code>token_url</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>client_id</code> | <code>string</code> | 必填 | 未注明 | Required. OAuth2 client ID. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>client_secret</code> | <code>string</code> | 必填；仅输入 | 未注明 | Required. Input only. OAuth2 client secret. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>refresh_token</code> | <code>string</code> | 必填；仅输入 | 未注明 | Required. Input only. OAuth2 refresh token. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>scopes</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. List of OAuth2 scopes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>token_url</code> | <code>string</code> | 必填 | 未注明 | Required. OAuth2 token endpoint URL for refreshing access tokens. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"oauth2"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## OAuth2UpdateConfig {#schema-oauth2updateconfig}

出现位置：请求体（可能同时用于输出）。

Configuration for updating OAuth2 credentials.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>client_id</code> | <code>string</code> | 可选 | 未注明 | Optional. OAuth2 client ID. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>client_secret</code> | <code>string</code> | 可选；仅输入 | 未注明 | Optional. Input only. OAuth2 client secret. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>refresh_token</code> | <code>string</code> | 可选；仅输入 | 未注明 | Optional. Input only. OAuth2 refresh token. Write-only; never returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>scopes</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. List of OAuth2 scopes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>token_url</code> | <code>string</code> | 可选 | 未注明 | Optional. OAuth2 token endpoint URL for refreshing access tokens. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"oauth2"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ParallelAISearchConfig {#schema-parallelaisearchconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Used to specify configuration for ParallelAISearch.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>api_key</code> | <code>string</code> | 可选 | 未注明 | Optional. The API key for ParallelAiSearch. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>custom_config</code> | Map&lt;string, <code>any</code>&gt; | 可选 | 未注明 | Optional. Custom configs for ParallelAiSearch. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## PingWebhookRequest {#schema-pingwebhookrequest}

出现位置：请求体（可能同时用于输出）。

Request message for WebhookService.PingWebhook.

结构 / 允许值：<code>object</code>。

## PingWebhookResponse {#schema-pingwebhookresponse}

出现位置：响应 / 错误 / SSE。

Response message for WebhookService.PingWebhook.

结构 / 允许值：<code>object</code>。

## Pitch {#schema-pitch}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"low"</code> / <code>"medium"</code> / <code>"high"</code>。

枚举含义（<code>$</code>）：

- <code>low</code>：Lower pitch voice.
- <code>medium</code>：Medium pitch voice.
- <code>high</code>：Higher pitch voice.

## PlaceCitation {#schema-placecitation}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A place citation annotation.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>end_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | End of the attributed segment, exclusive. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | Title of the place. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>place_id</code> | <code>string</code> | 可选 | 未注明 | The ID of the place, in `places/&#123;place_id&#125;` format. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>review_snippets</code> | Array&lt;[ReviewSnippet](#schema-reviewsnippet)&gt; | 可选 | 未注明 | Snippets of reviews that are used to generate answers about the features of a given place in Google Maps. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>start_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | Start of segment of the response that is attributed to this source.  Index indicates the start of the segment, measured in bytes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"place_citation"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 | URI reference of the place. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Places {#schema-places}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | Title of the place. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>place_id</code> | <code>string</code> | 可选 | 未注明 | The ID of the place, in `places/&#123;place_id&#125;` format. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>review_snippets</code> | Array&lt;[ReviewSnippet](#schema-reviewsnippet)&gt; | 可选 | 未注明 | Snippets of reviews that are used to generate answers about the features of a given place in Google Maps. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 | URI reference of the place. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ProcessingCallDelta {#schema-processingcalldelta}

出现位置：响应 / 错误 / SSE。

Streaming delta for a server-initiated media processing step.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"processing_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ProcessingCallStep {#schema-processingcallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A server-initiated processing step for media analysis (e.g. video understanding).

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"processing_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ProcessingResultDelta {#schema-processingresultdelta}

出现位置：响应 / 错误 / SSE。

Streaming delta for the result of a server-initiated media processing step.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"processing_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ProcessingResultStep {#schema-processingresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The result of a server-initiated media processing step.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"processing_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## PromptedVoice {#schema-promptedvoice}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Parameters for prompted voice generation. Required in `CreateVoice` when `type` is `"prompted"`. Returned in `CreateVoice`, `GetVoice`, and `ListVoices` responses for prompted voices.

结构 / 允许值：<code>object</code>。

官方 required：<code>input</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>input</code> | <code>string</code> | 必填 | 未注明 | Required. The natural-language prompt describing the desired voice, e.g. "A deep, booming male voice of a massive evil ogre in his middle years." | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## RagResource {#schema-ragresource}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The definition of the Rag resource.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>rag_corpus</code> | <code>string</code> | 可选 | 未注明 | Optional. RagCorpora resource name. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>rag_file_ids</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. rag_file_id. The files should be in the same rag_corpus set in rag_corpus field. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## RagRetrievalConfig {#schema-ragretrievalconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Specifies the context retrieval config.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>filter</code> | [Filter](#schema-filter) | 可选 | 未注明 | Optional. Config for filters. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>hybrid_search</code> | [HybridSearch](#schema-hybridsearch) | 可选 | 未注明 | Optional. Config for Hybrid Search. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>ranking</code> | [Ranking](#schema-ranking) | 可选 | 未注明 | Optional. Config for ranking and reranking. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>top_k</code> | <code>integer (int32)</code> | 可选 | 未注明 | Optional. The number of contexts to retrieve. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## RagStoreConfig {#schema-ragstoreconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Use to specify configuration for RAG Store.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>rag_resources</code> | Array&lt;[RagResource](#schema-ragresource)&gt; | 可选 | 未注明 | Optional. The representation of the rag source. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>rag_retrieval_config</code> | [RagRetrievalConfig](#schema-ragretrievalconfig) | 可选 | 未注明 | Optional. The retrieval config for the Rag query. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>similarity_top_k</code> | <code>integer (int32)</code> | 可选；废弃 | 未注明 | Optional. Number of top k results to return from the selected corpora. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>vector_distance_threshold</code> | <code>number (double)</code> | 可选；废弃 | 未注明 | Optional. Only return results with vector distance smaller than the threshold. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## RankService {#schema-rankservice}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Config for Rank Service.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>model_name</code> | <code>string</code> | 可选 | 未注明 | Optional. The model name of the rank service. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Ranking {#schema-ranking}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Config for ranking and reranking.

结构 / 允许值：<code>object</code>。

官方 required：<code>ranking_config</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>model_name</code> | <code>string</code> | 可选 | 未注明 | Optional. The model name of the rank service. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>rank_service</code> | [RankService](#schema-rankservice) | 可选 | 未注明 | Config for Rank Service. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>ranking_config</code> | <code>"rank_service"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ReplicatedVoice {#schema-replicatedvoice}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Input only. Parameters for replicated voice generation. Required on input when `type` is `"replicated"`. Not returned in responses.

结构 / 允许值：<code>object</code>。

官方 required：<code>consent_audio</code>、<code>source_audio</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>consent_audio</code> | [AudioData](#schema-audiodata) | 必填 | 未注明 | Required. Audio recording of the same speaker reading the required consent phrase, confirming authorization to replicate the voice. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>source_audio</code> | [AudioData](#schema-audiodata) | 必填 | 未注明 | Required. The reference audio sample of the speaker's voice to replicate. Must be an unmodified recording of an adult human voice. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ResponseFormat {#schema-responseformat}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：[AudioResponseFormat](#schema-audioresponseformat) / [ImageResponseFormat](#schema-imageresponseformat) / [TextResponseFormat](#schema-textresponseformat) / [VideoResponseFormat](#schema-videoresponseformat) / Map&lt;string, <code>any</code>&gt;。

## ResponseModality {#schema-responsemodality}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"text"</code> / <code>"image"</code> / <code>"audio"</code> / <code>"video"</code> / <code>"document"</code>。

枚举含义（<code>$</code>）：

- <code>text</code>：Indicates the model should return text.
- <code>image</code>：Indicates the model should return images.
- <code>audio</code>：Indicates the model should return audio.
- <code>video</code>：Indicates the model should return video.
- <code>document</code>：Indicates the model should return documents.

## Retrieval {#schema-retrieval}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model to retrieve files.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>exa_ai_search_config</code> | [ExaAISearchConfig](#schema-exaaisearchconfig) | 可选 | 未注明 | Used to specify configuration for ExaAISearch. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>parallel_ai_search_config</code> | [ParallelAISearchConfig](#schema-parallelaisearchconfig) | 可选 | 未注明 | Used to specify configuration for ParallelAISearch. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>rag_store_config</code> | [RagStoreConfig](#schema-ragstoreconfig) | 可选 | 未注明 | Used to specify configuration for RagStore. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>retrieval_types</code> | Array&lt;<code>"vertex_ai_search"</code> / <code>"rag_store"</code> / <code>"exa_ai_search"</code> / <code>"parallel_ai_search"</code>&gt; | 可选 | 未注明 | The types of file retrieval to enable. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"retrieval"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>vertex_ai_search_config</code> | [VertexAISearchConfig](#schema-vertexaisearchconfig) | 可选 | 未注明 | Used to specify configuration for VertexAISearch. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.retrieval_types.items</code>）：

- <code>vertex_ai_search</code>：官方未说明
- <code>rag_store</code>：官方未说明
- <code>exa_ai_search</code>：官方未说明
- <code>parallel_ai_search</code>：官方未说明

## RetrievalCallDelta {#schema-retrievalcalldelta}

出现位置：响应 / 错误 / SSE。

Used by Vertex Retrieval tools such as Parallel AI, Exa AI, Vertex AI Search, etc. RetrievalType decides which tool is used.

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [RetrievalStepArguments](#schema-retrievalsteparguments) | 必填 | 未注明 | Required. The arguments to pass to the Retrieval tool. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>retrieval_type</code> | <code>"vertex_ai_search"</code> / <code>"rag_store"</code> / <code>"exa_ai_search"</code> / <code>"parallel_ai_search"</code> | 可选 | 未注明 | The type of retrieval tools. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"retrieval_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.retrieval_type</code>）：

- <code>vertex_ai_search</code>：官方未说明
- <code>rag_store</code>：官方未说明
- <code>exa_ai_search</code>：官方未说明
- <code>parallel_ai_search</code>：官方未说明

## RetrievalCallStep {#schema-retrievalcallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Retrieval call step. Used by Vertex Retrieval tools such as Parallel AI, Exa AI, Vertex AI Search, etc. RetrievalType decides which tool is used.

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [RetrievalStepArguments](#schema-retrievalsteparguments) | 必填 | 未注明 | Required. The arguments to pass to the retrieval tool. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>retrieval_type</code> | <code>"vertex_ai_search"</code> / <code>"rag_store"</code> / <code>"exa_ai_search"</code> / <code>"parallel_ai_search"</code> | 可选 | 未注明 | The type of retrieval tools. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"retrieval_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.retrieval_type</code>）：

- <code>vertex_ai_search</code>：官方未说明
- <code>rag_store</code>：官方未说明
- <code>exa_ai_search</code>：官方未说明
- <code>parallel_ai_search</code>：官方未说明

## RetrievalResultDelta {#schema-retrievalresultdelta}

出现位置：响应 / 错误 / SSE。

Used by Vertex Retrieval tools such as Parallel AI, Exa AI, Vertex AI Search, etc. ToolResultDelta.type

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 | Whether the retrieval resulted in an error. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"retrieval_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## RetrievalResultStep {#schema-retrievalresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Vertex Retrieval result step. Used by Vertex Retrieval tools such as Parallel AI, Exa AI, Vertex AI Search, etc.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 | Whether the retrieval resulted in an error. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"retrieval_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## RetrievalStepArguments {#schema-retrievalsteparguments}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The arguments to pass to Retrieval tools.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>queries</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Queries for Retrieval information. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ReviewSnippet {#schema-reviewsnippet}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Encapsulates a snippet of a user review that answers a question about the features of a specific place in Google Maps.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>review_id</code> | <code>string</code> | 可选 | 未注明 | The ID of the review snippet. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>title</code> | <code>string</code> | 可选 | 未注明 | Title of the review. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 | A link that corresponds to the user review on Google Maps. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## RotateSigningSecretRequest {#schema-rotatesigningsecretrequest}

出现位置：请求体（可能同时用于输出）。

Request message for WebhookService.RotateSigningSecret.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>revocation_behavior</code> | <code>"revoke_previous_secrets_after_h24"</code> / <code>"revoke_previous_secrets_immediately"</code> | 可选 | 未注明 | Optional. The revocation behavior for previous signing secrets. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.revocation_behavior</code>）：

- <code>revoke_previous_secrets_after_h24</code>：Generate a new signing secret and revoke all previous secrets after 24 hours. Default and safest option for migrations.
- <code>revoke_previous_secrets_immediately</code>：Revoke all previous secrets immediately. Use with caution as this can interrupt ongoing notifications.

## RotateSigningSecretResponse {#schema-rotatesigningsecretresponse}

出现位置：响应 / 错误 / SSE。

Response message for WebhookService.RotateSigningSecret.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>secret</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The newly generated signing secret. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## RunTriggerRequest {#schema-runtriggerrequest}

出现位置：官方保留组件。

Request message for TriggerService.RunTrigger.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. The ID of the trigger to run immediately. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## SafetySetting {#schema-safetysetting}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A safety setting that affects the safety-blocking behavior.  A SafetySetting consists of a harm category and a threshold for that category.

结构 / 允许值：<code>object</code>。

官方 required：<code>threshold</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>method</code> | <code>"severity"</code> / <code>"probability"</code> | 可选 | 未注明 | Optional. The method for blocking content. If not specified, the default behavior is to use the probability score. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>threshold</code> | <code>"block_low_and_above"</code> / <code>"block_medium_and_above"</code> / <code>"block_only_high"</code> / <code>"block_none"</code> / <code>"off"</code> | 必填 | 未注明 | Required. The threshold for blocking content. If the harm probability exceeds this threshold, the content will be blocked. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | [HarmCategory](#schema-harmcategory) | 必填 | 未注明 | Required. The type of harm category to be blocked. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.method</code>）：

- <code>severity</code>：The harm block method uses both probability and severity scores.
- <code>probability</code>：The harm block method uses the probability score.

枚举含义（<code>$.properties.threshold</code>）：

- <code>block_low_and_above</code>：Block content with a low harm probability or higher.
- <code>block_medium_and_above</code>：Block content with a medium harm probability or higher.
- <code>block_only_high</code>：Block content with a high harm probability.
- <code>block_none</code>：Do not block any content, regardless of its harm probability.
- <code>off</code>：Turn off the safety filter entirely.

## ServiceTier {#schema-servicetier}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"flex"</code> / <code>"standard"</code> / <code>"priority"</code> / <code>"deferred"</code>。

枚举含义（<code>$</code>）：

- <code>flex</code>：Flex service tier.
- <code>standard</code>：Standard service tier.
- <code>priority</code>：Priority service tier.
- <code>deferred</code>：Deferred service tier.

## SessionConfig {#schema-sessionconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The configuration of CodeMender sessions.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>max_rounds</code> | <code>integer (int32)</code> | 可选 | 未注明 | The maximum number of interaction rounds the agent is allowed to perform before reaching a timeout. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## SigningSecret {#schema-signingsecret}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Represents a signing secret used to verify webhook payloads.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>expire_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The expiration date of the signing secret. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>truncated_secret</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The truncated version of the signing secret. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## SmartTranscriptionMode {#schema-smarttranscriptionmode}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for smart transcription mode.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>type</code> | <code>"smart"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Source {#schema-source}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A source to be mounted into the environment.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>content</code> | <code>string</code> | 可选 | 未注明 | The inline content if `type` is `INLINE`. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>encoding</code> | <code>string</code> | 可选 | 未注明 | Optional encoding for inline content (e.g. `base64`). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>source</code> | <code>string</code> | 可选 | 未注明 | The source of the environment. For Cloud Storage, this is the Cloud Storage path. For GitHub, this is the GitHub path. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>target</code> | <code>string</code> | 可选 | 未注明 | Where the source should appear in the environment. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"gcs"</code> / <code>"inline"</code> / <code>"repository"</code> / <code>"skill_registry"</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.type</code>）：

- <code>gcs</code>：A Cloud Storage bucket.
- <code>inline</code>：Inline content.
- <code>repository</code>：A generic repository. The protocol prefix in the source URL identifies the provider (e.g., github://, gcs://).
- <code>skill_registry</code>：A skill resource from the Skill Registry Service. Skill: projects/&#123;project&#125;/locations/&#123;location&#125;/skills/&#123;skill&#125; SkillRevision: projects/&#123;project&#125;/locations/&#123;location&#125;/skills/&#123;skill&#125;/revisions/&#123;revision&#125; Support mounting all skills under a project: projects/&#123;project&#125;/locations/&#123;location&#125;/skills.

## SpeakerConfig {#schema-speakerconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for multi-speaker and speech generation.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>speakers</code> | Array&lt;[SpeechConfig](#schema-speechconfig)&gt; | 可选 | 未注明 | Individual speaker configurations. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## SpeechAnnotation {#schema-speechannotation}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Speech annotation for text content.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>end_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | End of the attributed segment, exclusive. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>speaker</code> | <code>string</code> | 可选 | 未注明 | The speaker to associate with this turn. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>start_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | Start of segment of the response that is attributed to this source.  Index indicates the start of the segment, measured in bytes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>style</code> | <code>string</code> | 可选 | 未注明 | Style instruction for the speech synthesis. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"speech_metadata"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## SpeechConfig {#schema-speechconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The configuration for speech interaction.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>language</code> | <code>string</code> | 可选 | 未注明 | The language of the speech. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>speaker</code> | <code>string</code> | 可选 | 未注明 | The speaker's name, it should match the speaker name given in the prompt. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>voice</code> | <code>string</code> | 可选 | 未注明 | The voice of the speaker. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## StaticMediaProcessing {#schema-staticmediaprocessing}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>end_offset</code> | <code>string (google-duration)</code> | 可选 | 未注明 | Optional. Segment end time. Specified as a decimal number of seconds followed by an 's' suffix, e.g., "30s". Must be non-negative and greater than `start_offset` if `start_offset` is set. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>fps</code> | <code>number (double)</code> | 可选 | 未注明 | Optional. Video frame-rate sampling density. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>start_offset</code> | <code>string (google-duration)</code> | 可选 | 未注明 | Optional. Segment start time. Specified as a decimal number of seconds followed by an 's' suffix, e.g., "10.5s". Must be non-negative. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"static"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Status {#schema-status}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The `Status` type defines a logical error model that is suitable for different programming environments, including REST APIs and RPC APIs. It is used by [gRPC](https://github.com/grpc). Each `Status` message contains three pieces of data: error code, error message, and error details.  You can find out more about this error model and how to work with it in the [API Design Guide](https://cloud.google.com/apis/design/errors).

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>code</code> | <code>integer (int32)</code> | 可选 | 未注明 | The status code, which should be an enum value of google.rpc.Code. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>details</code> | Array&lt;Map&lt;string, <code>any</code>&gt;&gt; | 可选 | 未注明 | A list of messages that carry the error details.  There is a common set of message types for APIs to use. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>message</code> | <code>string</code> | 可选 | 未注明 | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the google.rpc.Status.details field, or localized by the client. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Step {#schema-step}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A step in the interaction.

结构 / 允许值：[CodeExecutionCallStep](#schema-codeexecutioncallstep) / [CodeExecutionResultStep](#schema-codeexecutionresultstep) / [FileSearchCallStep](#schema-filesearchcallstep) / [FileSearchResultStep](#schema-filesearchresultstep) / [FunctionCallStep](#schema-functioncallstep) / [FunctionResultStep](#schema-functionresultstep) / [GoogleMapsCallStep](#schema-googlemapscallstep) / [GoogleMapsResultStep](#schema-googlemapsresultstep) / [GoogleSearchCallStep](#schema-googlesearchcallstep) / [GoogleSearchResultStep](#schema-googlesearchresultstep) / [McpServerToolCallStep](#schema-mcpservertoolcallstep) / [McpServerToolResultStep](#schema-mcpservertoolresultstep) / [ModelOutputStep](#schema-modeloutputstep) / [ProcessingCallStep](#schema-processingcallstep) / [ProcessingResultStep](#schema-processingresultstep) / [RetrievalCallStep](#schema-retrievalcallstep) / [RetrievalResultStep](#schema-retrievalresultstep) / [ThoughtStep](#schema-thoughtstep) / [UrlContextCallStep](#schema-urlcontextcallstep) / [UrlContextResultStep](#schema-urlcontextresultstep) / [UserInputStep](#schema-userinputstep)。

## StepDelta {#schema-stepdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>delta</code>、<code>event_type</code>、<code>index</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>delta</code> | [StepDeltaData](#schema-stepdeltadata) | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_id</code> | <code>string</code> | 可选 | 未注明 | The event_id token to be used to resume the interaction stream, from this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_type</code> | <code>"step.delta"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>index</code> | <code>integer (int32)</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>metadata</code> | [StepDeltaMetadata](#schema-stepdeltametadata) | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## StepDeltaData {#schema-stepdeltadata}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：[ArgumentsDelta](#schema-argumentsdelta) / [AudioDelta](#schema-audiodelta) / [CodeExecutionCallDelta](#schema-codeexecutioncalldelta) / [CodeExecutionResultDelta](#schema-codeexecutionresultdelta) / [DocumentDelta](#schema-documentdelta) / [FileSearchCallDelta](#schema-filesearchcalldelta) / [FileSearchResultDelta](#schema-filesearchresultdelta) / [FunctionResultDelta](#schema-functionresultdelta) / [GoogleMapsCallDelta](#schema-googlemapscalldelta) / [GoogleMapsResultDelta](#schema-googlemapsresultdelta) / [GoogleSearchCallDelta](#schema-googlesearchcalldelta) / [GoogleSearchResultDelta](#schema-googlesearchresultdelta) / [ImageDelta](#schema-imagedelta) / [McpServerToolCallDelta](#schema-mcpservertoolcalldelta) / [McpServerToolResultDelta](#schema-mcpservertoolresultdelta) / [ProcessingCallDelta](#schema-processingcalldelta) / [ProcessingResultDelta](#schema-processingresultdelta) / [RetrievalCallDelta](#schema-retrievalcalldelta) / [RetrievalResultDelta](#schema-retrievalresultdelta) / [TextAnnotationDelta](#schema-textannotationdelta) / [TextDelta](#schema-textdelta) / [ThoughtSignatureDelta](#schema-thoughtsignaturedelta) / [ThoughtSummaryDelta](#schema-thoughtsummarydelta) / [UrlContextCallDelta](#schema-urlcontextcalldelta) / [UrlContextResultDelta](#schema-urlcontextresultdelta) / [VideoDelta](#schema-videodelta)。

## StepDeltaMetadata {#schema-stepdeltametadata}

出现位置：响应 / 错误 / SSE。

Optional metadata accompanying ANY streamed event.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>total_usage</code> | [Usage](#schema-usage) | 可选 | 未注明 | Statistics on the interaction request's token usage. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## StepStart {#schema-stepstart}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>event_type</code>、<code>index</code>、<code>step</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>event_id</code> | <code>string</code> | 可选 | 未注明 | The event_id token to be used to resume the interaction stream, from this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_type</code> | <code>"step.start"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>index</code> | <code>integer (int32)</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>step</code> | [Step](#schema-step) | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## StepStop {#schema-stepstop}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>event_type</code>、<code>index</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>event_id</code> | <code>string</code> | 可选 | 未注明 | The event_id token to be used to resume the interaction stream, from this event. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>event_type</code> | <code>"step.stop"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>index</code> | <code>integer (int32)</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>step_usage</code> | [Usage](#schema-usage) | 可选 | 未注明 | Model usage stats for this specific step. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>usage</code> | [Usage](#schema-usage) | 可选 | 未注明 | Cumulative model usage stats from the start of the session. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## TextAnnotationDelta {#schema-textannotationdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>annotations</code> | Array&lt;[Annotation](#schema-annotation)&gt; | 可选 | 未注明 | Citation information for model-generated content. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"text_annotation_delta"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## TextContent {#schema-textcontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A text content block.

结构 / 允许值：<code>object</code>。

官方 required：<code>text</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>annotations</code> | Array&lt;[Annotation](#schema-annotation)&gt; | 可选 | 未注明 | Citation information for model-generated content. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>text</code> | <code>string</code> | 必填 | 未注明 | Required. The text content. | 完整转发，由 Google 校验 | 部分：messages.content 文本 / 输出 content 文本 |
| <code>type</code> | <code>"text"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## TextDelta {#schema-textdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>text</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>text</code> | <code>string</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"text"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## TextResponseFormat {#schema-textresponseformat}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for text output format.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>mime_type</code> | <code>"application/json"</code> / <code>"text/plain"</code> | 可选 | 未注明 | The MIME type of the text output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>schema</code> | Map&lt;string, <code>any</code>&gt; | 可选 | 未注明 | The JSON schema that the output should conform to. Only applicable when mime_type is application/json. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"text"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>application/json</code>：JSON output format.
- <code>text/plain</code>：Plain text output format.

## ThinkingLevel {#schema-thinkinglevel}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"minimal"</code> / <code>"low"</code> / <code>"medium"</code> / <code>"high"</code>。

枚举含义（<code>$</code>）：

- <code>minimal</code>：Little to no thinking.
- <code>low</code>：Low thinking level.
- <code>medium</code>：Medium thinking level.
- <code>high</code>：High thinking level.

## ThinkingSummaries {#schema-thinkingsummaries}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"auto"</code> / <code>"none"</code>。

枚举含义（<code>$</code>）：

- <code>auto</code>：Auto thinking summaries.
- <code>none</code>：No thinking summaries.

## ThoughtContent {#schema-thoughtcontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：[ImageContent](#schema-imagecontent) / [TextContent](#schema-textcontent)。

## ThoughtSignatureDelta {#schema-thoughtsignaturedelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | Signature to match the backend source to be part of the generation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"thought_signature"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## ThoughtStep {#schema-thoughtstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A thought step.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>summary</code> | Array&lt;[ThoughtContent](#schema-thoughtcontent)&gt; | 可选 | 未注明 | A summary of the thought. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"thought"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## ThoughtSummaryDelta {#schema-thoughtsummarydelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>content</code> | [Content](#schema-content) | 可选 | 未注明 | A new summary item to be added to the thought. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"thought_summary"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## Tool {#schema-tool}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model.

结构 / 允许值：[CodeExecution](#schema-codeexecution) / [ComputerUse](#schema-computeruse) / [FileSearch](#schema-filesearch) / [Function](#schema-function) / [GoogleMaps](#schema-googlemaps) / [GoogleSearch](#schema-googlesearch) / [McpServer](#schema-mcpserver) / [Retrieval](#schema-retrieval) / [UrlContext](#schema-urlcontext)。

## ToolChoiceConfig {#schema-toolchoiceconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The tool choice configuration containing allowed tools.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>allowed_tools</code> | [AllowedTools](#schema-allowedtools) | 可选 | 未注明 | The allowed tools. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## TranscriptionConfig {#schema-transcriptionconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for speech recognition (transcription).

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>adaptation_phrases</code> | Array&lt;<code>string</code>&gt; | 可选；废弃 | 未注明 | Optional. A list of phrases to bias the ASR model towards. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>custom_vocabulary</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. A list of custom vocabulary phrases to bias the speech recognition model toward recognizing specific terms. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>diarization_mode</code> | <code>string</code> | 可选；废弃 | 未注明 | Optional. Configures speaker diarization. Supported values: "speaker". | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>language_codes</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. BCP-47 language codes providing hints about the languages present in the audio. If omitted or empty, defaults to automatic language detection. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>language_hints</code> | Array&lt;<code>string</code>&gt; | 可选；废弃 | 未注明 | Deprecated: use language_codes. BCP-47 language codes providing hints about the languages present in the audio. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mode</code> | [TranscriptionMode](#schema-transcriptionmode) / <code>"verbatim"</code> / <code>"smart"</code> | 可选 | 未注明 | Discriminated transcription mode options or enum. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>timestamp_granularities</code> | Array&lt;<code>string</code>&gt; | 可选；废弃 | 未注明 | Optional. The granularity of timestamps to include in the transcription output. Supported values: "word". If empty, no timestamps are generated. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mode.oneOf.1</code>）：

- <code>verbatim</code>：Verbatim transcription mode.
- <code>smart</code>：Smart transcription mode.

## TranscriptionMode {#schema-transcriptionmode}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for transcription mode.

结构 / 允许值：[SmartTranscriptionMode](#schema-smarttranscriptionmode) / [VerbatimTranscriptionMode](#schema-verbatimtranscriptionmode)。

## Trigger {#schema-trigger}

出现位置：响应 / 错误 / SSE。

A trigger configuration that is scheduled to run an agent.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>interaction</code>、<code>schedule</code>、<code>time_zone</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>consecutive_failure_count</code> | <code>integer (int32)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The number of consecutive failures that have occurred since the last successful execution. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>create_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the trigger was created. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>display_name</code> | <code>string</code> | 可选 | 未注明 | Optional. The display name of the trigger. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>environment_id</code> | <code>string</code> | 可选 | 未注明 | Optional. The environment ID for the trigger execution. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>execution_timeout_seconds</code> | <code>integer (int32)</code> | 可选 | 未注明 | Optional. The execution timeout for the triggered interaction. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>id</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. Identifier. The ID of the trigger. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>interaction</code> | [AgentInteraction](#schema-agentinteraction) | 必填 | 未注明 | Required. The interaction request template to be executed. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>last_pause_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the trigger was last paused. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>last_resume_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the trigger was last resumed. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>last_run_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the trigger was last run. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>max_consecutive_failures</code> | <code>integer (int32)</code> | 可选 | 未注明 | Optional. The maximum number of consecutive failures allowed before the trigger is automatically paused (status becomes ERROR). | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>next_run_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the trigger is scheduled to run next. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>previous_interaction_id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The ID of the last interaction created by this trigger. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>schedule</code> | <code>string</code> | 必填 | 未注明 | Required. The cron schedule on which the trigger should run. Standard cron format. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>status</code> | <code>"active"</code> / <code>"paused"</code> / <code>"error"</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The current status of the trigger. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>time_zone</code> | <code>string</code> | 必填 | 未注明 | Required. Time zone in which the schedule should be interpreted. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>update_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the trigger was last updated. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.status</code>）：

- <code>active</code>：The trigger is active and will fire on schedule.
- <code>paused</code>：The trigger is paused and will not fire.
- <code>error</code>：The trigger has entered an error state due to consecutive failures.

## TriggerCreateParams {#schema-triggercreateparams}

出现位置：请求体（可能同时用于输出）。

Parameters for creating a trigger.

结构 / 允许值：<code>object</code>。

官方 required：<code>interaction</code>、<code>schedule</code>、<code>time_zone</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>display_name</code> | <code>string</code> | 可选 | 未注明 | Optional. The display name of the trigger. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>environment_id</code> | <code>string</code> | 可选 | 未注明 | Optional. The environment ID for the trigger execution. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>execution_timeout_seconds</code> | <code>integer (int32)</code> | 可选 | 未注明 | Optional. The execution timeout for the triggered interaction. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>interaction</code> | [AgentInteraction](#schema-agentinteraction) | 必填 | 未注明 | Required. The interaction request template to be executed. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>max_consecutive_failures</code> | <code>integer (int32)</code> | 可选 | 未注明 | Optional. The maximum number of consecutive failures allowed before the trigger is automatically paused (status becomes ERROR). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>schedule</code> | <code>string</code> | 必填 | 未注明 | Required. The cron schedule on which the trigger should run. Standard cron format. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>time_zone</code> | <code>string</code> | 必填 | 未注明 | Required. Time zone in which the schedule should be interpreted. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## TriggerExecution {#schema-triggerexecution}

出现位置：响应 / 错误 / SSE。

An execution instance of a trigger.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>trigger_id</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>end_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the execution finished. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>environment_id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The environment ID used for the execution. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>error</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The error message if the execution failed. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>id</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. Identifier. The ID of the trigger execution. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>interaction_id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The ID of the interaction created by this execution, if any. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>scheduled_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the execution was scheduled to run. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>start_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The time when the execution started. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>status</code> | <code>"in_progress"</code> / <code>"completed"</code> / <code>"failed"</code> / <code>"skipped"</code> / <code>"timed_out"</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The status of the execution. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>trigger_id</code> | <code>string</code> | 必填；只读；官方描述：仅输出 | 未注明 | Required. Output only. Identifier. The ID of the trigger that created this execution. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.status</code>）：

- <code>in_progress</code>：The execution is currently in progress.
- <code>completed</code>：The execution completed successfully.
- <code>failed</code>：The execution failed.
- <code>skipped</code>：The execution was skipped (e.g., previous execution still running).
- <code>timed_out</code>：The execution timed out.

## TriggerUpdate {#schema-triggerupdate}

出现位置：请求体（可能同时用于输出）。

Represents the fields of a Trigger that can be updated.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>display_name</code> | <code>string</code> | 可选 | 未注明 | Optional. The display name of the trigger. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>status</code> | <code>"active"</code> / <code>"paused"</code> / <code>"error"</code> | 可选 | 未注明 | Optional. The status of the trigger. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.status</code>）：

- <code>active</code>：The trigger is active and will fire on schedule.
- <code>paused</code>：The trigger is paused and will not fire.
- <code>error</code>：The trigger has entered an error state due to consecutive failures.

## UpdateTriggerRequest {#schema-updatetriggerrequest}

出现位置：官方保留组件。

Request message for TriggerService.UpdateTrigger.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>trigger</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. The ID of the trigger to update. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>trigger</code> | [TriggerUpdate](#schema-triggerupdate) | 必填 | 未注明 | Required. The trigger to update. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>update_mask</code> | <code>string (google-fieldmask)</code> | 可选 | <code>pattern="^(\\s*[^,\\s.]+(\\s*[,.]\\s*[^,\\s.]+)*)?$"</code> | Optional. The update mask applies to the resource. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## UpdateWebhookRequest {#schema-updatewebhookrequest}

出现位置：官方保留组件。

Request message for WebhookService.UpdateWebhook.

结构 / 允许值：<code>object</code>。

官方 required：<code>id</code>、<code>webhook</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. The ID of the webhook to update. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>update_mask</code> | <code>string (google-fieldmask)</code> | 可选 | <code>pattern="^(\\s*[^,\\s.]+(\\s*[,.]\\s*[^,\\s.]+)*)?$"</code> | Optional. The list of fields to update. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>webhook</code> | [WebhookUpdate](#schema-webhookupdate) | 必填 | 未注明 | Required. The webhook to update. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## UploadEnvironmentFileRequest {#schema-uploadenvironmentfilerequest}

出现位置：官方保留组件。

Request for `UploadEnvironmentFile`.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>extract</code> | <code>boolean</code> | 可选 | 未注明 | Optional. If true, treats the uploaded file as a tar/tar.gz archive and unpacks it into `path`. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>overwrite</code> | <code>boolean</code> | 可选 | 未注明 | Optional. Whether to overwrite the destination file if it already exists. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## UploadEnvironmentFileResponse {#schema-uploadenvironmentfileresponse}

出现位置：官方保留组件。

Response for `UploadEnvironmentFile`.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>files</code> | Array&lt;[EnvironmentFile](#schema-environmentfile)&gt; | 可选 | 未注明 | List of files created or extracted in the environment. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## UrlCitation {#schema-urlcitation}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A URL citation annotation.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>end_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | End of the attributed segment, exclusive. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>start_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | Start of segment of the response that is attributed to this source.  Index indicates the start of the segment, measured in bytes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>title</code> | <code>string</code> | 可选 | 未注明 | The title of the URL. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"url_citation"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 | The URL. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## UrlContext {#schema-urlcontext}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A tool that can be used by the model to fetch URL context.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>type</code> | <code>"url_context"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## UrlContextCallArguments {#schema-urlcontextcallarguments}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The arguments to pass to the URL context.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>urls</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | The URLs to fetch. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## UrlContextCallDelta {#schema-urlcontextcalldelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [UrlContextCallArguments](#schema-urlcontextcallarguments) | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"url_context_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## UrlContextCallStep {#schema-urlcontextcallstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

URL context call step.

结构 / 允许值：<code>object</code>。

官方 required：<code>arguments</code>、<code>id</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>arguments</code> | [UrlContextCallArguments](#schema-urlcontextcallarguments) | 必填 | 未注明 | Required. The arguments to pass to the URL context. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 必填 | 未注明 | Required. A unique ID for this specific tool call. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"url_context_call"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## UrlContextCallStepArguments {#schema-urlcontextcallsteparguments}

出现位置：官方保留组件。

The arguments to pass to the URL context.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>urls</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | The URLs to fetch. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## UrlContextResult {#schema-urlcontextresult}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

The result of the URL context.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>status</code> | <code>"success"</code> / <code>"error"</code> / <code>"paywall"</code> / <code>"unsafe"</code> | 可选 | 未注明 | The status of the URL retrieval. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 | The URL that was fetched. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.status</code>）：

- <code>success</code>：Url retrieval is successful.
- <code>error</code>：Url retrieval is failed due to error.
- <code>paywall</code>：Url retrieval is failed because the content is behind paywall.
- <code>unsafe</code>：Url retrieval is failed because the content is unsafe.

## UrlContextResultDelta {#schema-urlcontextresultdelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>result</code> | Array&lt;[UrlContextResult](#schema-urlcontextresult)&gt; | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"url_context_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

## UrlContextResultItem {#schema-urlcontextresultitem}

出现位置：官方保留组件。

The result of the URL context.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>status</code> | <code>"success"</code> / <code>"error"</code> / <code>"paywall"</code> / <code>"unsafe"</code> | 可选 | 未注明 | The status of the URL retrieval. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>url</code> | <code>string</code> | 可选 | 未注明 | The URL that was fetched. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.status</code>）：

- <code>success</code>：官方未说明
- <code>error</code>：官方未说明
- <code>paywall</code>：官方未说明
- <code>unsafe</code>：官方未说明

## UrlContextResultStep {#schema-urlcontextresultstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

URL context result step.

结构 / 允许值：<code>object</code>。

官方 required：<code>call_id</code>、<code>result</code>、<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>call_id</code> | <code>string</code> | 必填 | 未注明 | Required. ID to match the ID from the function call block. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>is_error</code> | <code>boolean</code> | 可选 | 未注明 | Whether the URL context resulted in an error. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>result</code> | Array&lt;[UrlContextResult](#schema-urlcontextresult)&gt; | 必填 | 未注明 | Required. The results of the URL context. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signature</code> | <code>string (byte)</code> | 可选 | 未注明 | A signature hash for backend validation. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"url_context_result"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## Usage {#schema-usage}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Statistics on the interaction request's token usage.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>cached_tokens_by_modality</code> | Array&lt;[ModalityTokens](#schema-modalitytokens)&gt; | 可选 | 未注明 | A breakdown of cached token usage by modality. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>grounding_tool_count</code> | Array&lt;[GroundingToolCount](#schema-groundingtoolcount)&gt; | 可选 | 未注明 | Grounding tool count. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>input_tokens_by_modality</code> | Array&lt;[ModalityTokens](#schema-modalitytokens)&gt; | 可选 | 未注明 | A breakdown of input token usage by modality. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>output_tokens_by_modality</code> | Array&lt;[ModalityTokens](#schema-modalitytokens)&gt; | 可选 | 未注明 | A breakdown of output token usage by modality. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>tool_use_tokens_by_modality</code> | Array&lt;[ModalityTokens](#schema-modalitytokens)&gt; | 可选 | 未注明 | A breakdown of tool-use token usage by modality. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>total_cached_tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | Number of tokens in the cached part of the prompt (the cached content). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>total_input_tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | Number of tokens in the prompt (context). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>total_output_tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | Total number of tokens across all the generated responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>total_thought_tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | Number of tokens of thoughts for thinking models. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>total_tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | Total token count for the interaction request (prompt + responses + other internal tokens). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>total_tool_use_tokens</code> | <code>integer (int32)</code> | 可选 | 未注明 | Number of tokens present in tool-use prompt(s). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## UserInputStep {#schema-userinputstep}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Input provided by the user.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>content</code> | Array&lt;[Content](#schema-content)&gt; | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"user_input"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## VerbatimTranscriptionMode {#schema-verbatimtranscriptionmode}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for verbatim transcription mode.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>diarization_mode</code> | <code>string</code> | 可选 | 未注明 | Optional. Configures speaker diarization. Supported values: "speaker". | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>timestamp_granularities</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. The granularity of timestamps to include in the transcription output. Supported values: "word". If empty, no timestamps are generated. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"verbatim"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## VertexAISearchConfig {#schema-vertexaisearchconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Used to specify configuration for VertexAISearch.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>datastores</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. Used to specify Vertex AI Search datastores. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>engine</code> | <code>string</code> | 可选 | 未注明 | Optional. Used to specify Vertex AI Search engine. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## VideoConfig {#schema-videoconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration options for video generation.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>task</code> | <code>"text_to_video"</code> / <code>"image_to_video"</code> / <code>"reference_to_video"</code> / <code>"edit"</code> / <code>"extend"</code> | 可选 | 未注明 | Optional task mode for video generation. If not specified, the model automatically determines the appropriate mode based on the provided text prompt and input media. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.task</code>）：

- <code>text_to_video</code>：Generates video solely from a text prompt.
- <code>image_to_video</code>：Generates video from one or two source images. The first image defines the starting frame, and the optional second image defines the ending frame.
- <code>reference_to_video</code>：Generates video using reference media (such as images, audio, or video).
- <code>edit</code>：Modifies an existing input video.
- <code>extend</code>：Extends an existing input video.

## VideoContent {#schema-videocontent}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A video content block.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 | The video content. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>mime_type</code> | <code>"video/mp4"</code> / <code>"video/mpeg"</code> / <code>"video/mpg"</code> / <code>"video/mov"</code> / <code>"video/avi"</code> / <code>"video/x-flv"</code> / <code>"video/webm"</code> / <code>"video/wmv"</code> / <code>"video/3gpp"</code> | 可选 | 未注明 | The mime type of the video. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | A user-defined name for this content block. Can be referenced by the model in the final response. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>processing</code> | [MediaProcessing](#schema-mediaprocessing) / <code>"static"</code> / <code>"agentic"</code> | 可选 | 未注明 | How the model processes this video for understanding. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>resolution</code> | [MediaResolution](#schema-mediaresolution) | 可选 | 未注明 | The resolution of the media. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"video"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 | The URI of the video. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>video/mp4</code>：MP4 video format
- <code>video/mpeg</code>：MPEG video format
- <code>video/mpg</code>：MPG video format
- <code>video/mov</code>：MOV video format
- <code>video/avi</code>：AVI video format
- <code>video/x-flv</code>：FLV video format
- <code>video/webm</code>：WebM video format
- <code>video/wmv</code>：WMV video format
- <code>video/3gpp</code>：3GPP video format

枚举含义（<code>$.properties.processing.oneOf.1</code>）：

- <code>static</code>：Fixed-rate frame extraction. All frames placed in context.
- <code>agentic</code>：Model-driven dynamic navigation.

## VideoDelta {#schema-videodelta}

出现位置：响应 / 错误 / SSE。

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>data</code> | <code>string (byte)</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>mime_type</code> | <code>"video/mp4"</code> / <code>"video/mpeg"</code> / <code>"video/mpg"</code> / <code>"video/mov"</code> / <code>"video/avi"</code> / <code>"video/x-flv"</code> / <code>"video/webm"</code> / <code>"video/wmv"</code> / <code>"video/3gpp"</code> / <code>"video/jpeg2000"</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>resolution</code> | [MediaResolution](#schema-mediaresolution) | 可选 | 未注明 | The resolution of the media. | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>type</code> | <code>"video"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 |  | 完整转发，由 Google 校验 | 未返回此结构：网关只返回聊天文本/媒体链接、兼容 usage 或 Veo 任务 |

枚举含义（<code>$.properties.mime_type</code>）：

- <code>video/mp4</code>：MP4 video format
- <code>video/mpeg</code>：MPEG video format
- <code>video/mpg</code>：MPG video format
- <code>video/mov</code>：MOV video format
- <code>video/avi</code>：AVI video format
- <code>video/x-flv</code>：FLV video format
- <code>video/webm</code>：WebM video format
- <code>video/wmv</code>：WMV video format
- <code>video/3gpp</code>：3GPP video format
- <code>video/jpeg2000</code>：JPEG 2000 video format

## VideoResponseFormat {#schema-videoresponseformat}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Configuration for video output format.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>aspect_ratio</code> | <code>"16:9"</code> / <code>"9:16"</code> | 可选 | 未注明 | The aspect ratio for the video output. | 完整转发，由 Google 校验 | 相似能力：Veo aspect_ratio / resolution / seconds；没有此对象映射 |
| <code>delivery</code> | <code>"inline"</code> / <code>"uri"</code> | 可选 | 未注明 | The delivery mode for the video output. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>duration</code> | <code>string (google-duration)</code> | 可选 | 未注明 | The duration for the video output. | 完整转发，由 Google 校验 | 相似能力：Veo aspect_ratio / resolution / seconds；没有此对象映射 |
| <code>gcs_uri</code> | <code>string</code> | 可选 | 未注明 | The Cloud Storage URI to store the video output. Required for Vertex if delivery mode is URI. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>resolution</code> | <code>"360p"</code> / <code>"720p"</code> / <code>"1080p"</code> / <code>"4k"</code> | 可选 | 未注明 | The video output resolution. Defaults to 720p. | 完整转发，由 Google 校验 | 相似能力：Veo aspect_ratio / resolution / seconds；没有此对象映射 |
| <code>type</code> | <code>"video"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.aspect_ratio</code>）：

- <code>16:9</code>：16:9 aspect ratio.
- <code>9:16</code>：9:16 aspect ratio.

枚举含义（<code>$.properties.delivery</code>）：

- <code>inline</code>：Video data is returned inline in the response.
- <code>uri</code>：Video data is returned as a URI.

枚举含义（<code>$.properties.resolution</code>）：

- <code>360p</code>：360p resolution.
- <code>720p</code>：720p resolution.
- <code>1080p</code>：1080p resolution.
- <code>4k</code>：4K resolution.

## Voice {#schema-voice}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A voice resource representing either a custom voice (created via `CreateVoice`) or a prebuilt system voice (returned by `ListVoices`).

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>accent</code> | <code>string</code> | 可选 | 未注明 | Optional. Regional accent descriptor (e.g. "American", "British"). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>context</code> | <code>string</code> | 可选 | 未注明 | Optional. Optimal usage context or domain (e.g. "Conversational", "Audiobook", "News"). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>description</code> | <code>string</code> | 可选 | 未注明 | Optional. Descriptive summary of vocal timbre, personality, and tone. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>display_name</code> | <code>string</code> | 可选 | 未注明 | Optional. User-provided display name for a stored voice (`store = true`), or the catalog name for a prebuilt voice. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>expire_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The timestamp at which a custom stored voice (`store = true`) or replicated voice key (`store = false`) expires. For custom stored voices (`store = true`), this expiration time is extended when the voice is used for speech synthesis or as a `base_voice` in `CreateVoice`. Unset for prebuilt catalog voices (`"prebuilt"`), which do not expire. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>gender</code> | <code>string</code> | 可选 | 未注明 | Optional. Perceived voice gender presentation (e.g. "female", "male", "neutral"). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The unique identifier of the voice.  * For Google-managed custom voices (`store = true`), this is a generated ID   with prefix `voice_` (for example, `voice_abc123def456`). Pass   `voices/&#123;id&#125;` as the `name` in `GetVoice` and `DeleteVoice`, and pass   `&#123;id&#125;` directly to `SpeechConfig.voice_config.voice` (or   `SpeechConfig.voice`) during speech synthesis. * For prebuilt catalog voices (`"prebuilt"` returned by `ListVoices`), this   is the speaker name (for example, `Puck` or `Charon`). * Unset when `CreateVoice` is called with `store = false`. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>key</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The client-managed voice replication key (with prefix `voicekey_`). Returned only by `CreateVoice` when `type` is `"replicated"` and `store` is `false`. Pass this key to `SpeechConfig.voice_config.voice` (or `SpeechConfig.voice`) during speech synthesis. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>language_code</code> | <code>string</code> | 可选 | 未注明 | Optional. Primary BCP-47 language tag (e.g. "en-US", "fr-FR"). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>model</code> | <code>string</code> | 可选 | 未注明 | Optional. The model used to design or replicate the voice. If omitted in `CreateVoice`, defaults to the latest supported voice design model. Returned in `CreateVoice`, `GetVoice`, and `ListVoices` responses for custom voices (`"prompted"` and `"replicated"`); unset for `"prebuilt"` voices. Created voices can be synthesized across any supported TTS synthesis model. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>persona</code> | <code>string</code> | 可选 | 未注明 | Optional. Intended persona or character archetype (e.g. "Warm, Friendly", "Narrator"). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>pitch</code> | [Pitch](#schema-pitch) | 可选 | 未注明 | Optional. Voice pitch classification. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>prompted</code> | [PromptedVoice](#schema-promptedvoice) | 可选 | 未注明 | Parameters for prompted voice generation. Required in `CreateVoice` when `type` is `"prompted"`. Returned in `CreateVoice`, `GetVoice`, and `ListVoices` responses for prompted voices. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>region_code</code> | <code>string</code> | 可选 | 未注明 | Optional. ISO 3166-1 alpha-2 or UN M.49 geographic region code (e.g. "US", "GB", "001"). | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>replicated</code> | [ReplicatedVoice](#schema-replicatedvoice) | 可选；仅输入 | 未注明 | Input only. Parameters for replicated voice generation. Required on input when `type` is `"replicated"`. Not returned in responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>sample_audio</code> | [AudioData](#schema-audiodata) | 可选；只读；官方描述：仅输出 | 未注明 | Output only. Sample audio (synthesizer prompt audio) generated for a prompted voice. Populated only in `CreateVoice` and `GetVoice` responses when `type` is `"prompted"`; unset in `ListVoices` responses and for `"replicated"` or `"prebuilt"` voices. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | [VoiceType](#schema-voicetype) | 必填 | 未注明 | Required. The type of the voice. In `CreateVoice`, must be `"replicated"` or `"prompted"`. In `ListVoices` responses, may also be `"prebuilt"`. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>usage</code> | [Usage](#schema-usage) | 可选；只读；官方描述：仅输出 | 未注明 | Output only. Token usage statistics for the voice creation request. Populated only in the response of `CreateVoice` when `type` is `"prompted"`; unset for `"replicated"` and in `GetVoice` and `ListVoices` responses. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## VoiceType {#schema-voicetype}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

结构 / 允许值：<code>"replicated"</code> / <code>"prompted"</code> / <code>"prebuilt"</code>。

枚举含义（<code>$</code>）：

- <code>replicated</code>：A custom voice replicated from a reference audio sample and speaker consent recording.
- <code>prompted</code>：A custom voice generated from a natural-language text prompt (optionally editing or remixing a `base_voice`).
- <code>prebuilt</code>：A built-in system voice from Google's voice catalog (e.g., `Puck`, `Charon`). Returned in responses; cannot be specified as the type in `CreateVoice`.

## Webhook {#schema-webhook}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

A Webhook resource.

结构 / 允许值：<code>object</code>。

官方 required：<code>subscribed_events</code>、<code>uri</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>create_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The timestamp when the webhook was created. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>id</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The ID of the webhook. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | Optional. The user-provided name of the webhook. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>new_signing_secret</code> | <code>string</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The new signing secret for the webhook. Only populated on create. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>signing_secrets</code> | Array&lt;[SigningSecret](#schema-signingsecret)&gt; | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The signing secrets associated with this webhook. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>state</code> | <code>"enabled"</code> / <code>"disabled"</code> / <code>"disabled_due_to_failed_deliveries"</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The state of the webhook. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>subscribed_events</code> | Array&lt;<code>"batch.succeeded"</code> / <code>"batch.expired"</code> / <code>"batch.failed"</code> / <code>"interaction.requires_action"</code> / <code>"interaction.completed"</code> / <code>"interaction.failed"</code> / <code>"video.generated"</code>&gt; | 必填 | 未注明 | Required. The events that the webhook is subscribed to. Available events: - batch.succeeded - batch.expired - batch.failed - interaction.requires_action - interaction.completed - interaction.failed - video.generated | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>update_time</code> | <code>string (date-time)</code> | 可选；只读；官方描述：仅输出 | 未注明 | Output only. The timestamp when the webhook was last updated. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>uri</code> | <code>string</code> | 必填 | 未注明 | Required. The URI to which webhook events will be sent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.state</code>）：

- <code>enabled</code>：The webhook is enabled.
- <code>disabled</code>：The webhook is disabled by the user.
- <code>disabled_due_to_failed_deliveries</code>：The webhook is disabled due to failed deliveries.

## WebhookConfig {#schema-webhookconfig}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Message for configuring webhook events for a request.

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>uris</code> | Array&lt;<code>string</code>&gt; | 可选 | 未注明 | Optional. If set, these webhook URIs will be used for webhook events instead of the registered webhooks. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>user_metadata</code> | Map&lt;string, <code>any</code>&gt; | 可选 | 未注明 | Optional. The user metadata that will be returned on each event emission to the webhooks. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

## WebhookUpdate {#schema-webhookupdate}

出现位置：请求体（可能同时用于输出）。

结构 / 允许值：<code>object</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>name</code> | <code>string</code> | 可选 | 未注明 | Optional. The user-provided name of the webhook. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>state</code> | <code>"enabled"</code> / <code>"disabled"</code> / <code>"disabled_due_to_failed_deliveries"</code> | 可选 | 未注明 | Optional. The state of the webhook. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>subscribed_events</code> | Array&lt;<code>"batch.succeeded"</code> / <code>"batch.expired"</code> / <code>"batch.failed"</code> / <code>"interaction.requires_action"</code> / <code>"interaction.completed"</code> / <code>"interaction.failed"</code> / <code>"video.generated"</code>&gt; | 可选 | 未注明 | Optional. The events that the webhook is subscribed to. Available events: - batch.succeeded - batch.expired - batch.failed - interaction.requires_action - interaction.completed - interaction.failed - video.generated | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>uri</code> | <code>string</code> | 可选 | 未注明 | Optional. The URI to which webhook events will be sent. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

枚举含义（<code>$.properties.state</code>）：

- <code>enabled</code>：The webhook is enabled.
- <code>disabled</code>：The webhook is disabled by the user.
- <code>disabled_due_to_failed_deliveries</code>：The webhook is disabled due to failed deliveries.

## WordInfo {#schema-wordinfo}

出现位置：请求体（可能同时用于输出）；响应 / 错误 / SSE。

Word-level ASR annotation for transcription output. Carries the word text, optional timing, and optional speaker attribution.

结构 / 允许值：<code>object</code>。

官方 required：<code>type</code>。

| 字段 | 类型 / 允许值 | 必填 / 只读 / 废弃 | 默认值 / 约束 | 官方说明 | 官方后端 | 网页登录态 / OpenAI 聊天 |
| --- | --- | --- | --- | --- | --- | --- |
| <code>end_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | End of the attributed segment, exclusive. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>end_offset</code> | <code>string (google-duration)</code> | 可选 | 未注明 | End offset in time of the word relative to the start of the audio. Present when timestamp_granularities contains "word". | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>speaker</code> | <code>string</code> | 可选 | 未注明 | Optional. Speaker label for this word (e.g. "spk_1", "spk_2"). Present when diarization_mode is set in TranscriptionConfig. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>start_index</code> | <code>integer (int32)</code> | 可选 | 未注明 | Start of segment of the response that is attributed to this source.  Index indicates the start of the segment, measured in bytes. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>start_offset</code> | <code>string (google-duration)</code> | 可选 | 未注明 | Start offset in time of the word relative to the start of the audio. Present when timestamp_granularities contains "word". | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>text</code> | <code>string</code> | 可选 | 未注明 | The transcribed word. | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |
| <code>type</code> | <code>"word_info"</code> | 必填 | 未注明 |  | 完整转发，由 Google 校验 | 未开放：聊天 HTTP 不接收此官方结构；见差距说明 |

