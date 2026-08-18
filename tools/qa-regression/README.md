# 问答链路回归工具

该工具用于持续验证统一问答链路，并与内置智能推理进行同题对比。两个入口共用同一份题库和配置：

- `run-unified.ps1`：只执行统一问答，适合日常快速回归。
- `run-comparison.ps1`：每道题依次执行统一问答和智能推理，适合比较回答覆盖、引用和速度。

路由验证不再直接调用或模拟路由模型，而是读取统一问答真实运行记录中的 `selected_agent_ids` 和 `route_outcome`，因此覆盖完整应用链路。

## 题库配置

编辑 `questions.json` 即可增删问题。每道题支持：

```json
{
  "id": "Q01",
  "enabled": true,
  "tags": ["finance", "travel_expense"],
  "question": "出差期间的餐费是否可以报销？",
  "expected": {
    "route": {
      "selected_agent_ids": ["finance"]
    },
    "unified": {
      "allowed_status": ["complete", "partial"],
      "min_references": 1,
      "is_fallback": false,
      "max_latency_ms": 180000,
      "must_contain": ["餐费"],
      "must_not_contain": ["内部错误"]
    }
  }
}
```

`route.outcome` 只需在 `out_of_service` 等需要明确检查的题目中配置。标签筛选采用“命中任一标签”。

## 运行配置

`config.json` 保存非敏感运行配置。统一问答和智能推理当前默认使用同一个固定测试模型 ID。可以通过 `UNIFIED_QA_TEST_MODEL_ID` 和 `UNIFIED_QA_TEST_SMART_MODEL_ID` 临时覆盖。访问令牌、JWT 密钥和数据库密码等敏感值只通过环境变量提供，不能写入配置或题库。

最简单的方式是只提供已有访问令牌：

```powershell
$env:UNIFIED_QA_TEST_ACCESS_TOKEN = "<access-token>"
```

如需临时改用其他模型，再设置下列变量：

```powershell
$env:UNIFIED_QA_TEST_MODEL_ID = "<model-id>"
$env:UNIFIED_QA_TEST_SMART_MODEL_ID = "<model-id>"
```

如果没有访问令牌，工具也可以为本地/测试环境生成临时 JWT，并写入 `auth_tokens`。运行结束后会自动删除临时记录：

```powershell
$env:UNIFIED_QA_TEST_USER_ID = "<test-user-id>"
$env:UNIFIED_QA_TEST_JWT_SECRET = "<jwt-secret>"
$env:UNIFIED_QA_TEST_DB_HOST = "<database-host>"
$env:UNIFIED_QA_TEST_DB_PASSWORD = "<database-password>"
```

`builtin-smart-reasoning` 已持久绑定到上述固定测试模型，`temporarily_bind_model` 默认为 `false`，因此对比测试不再修改或恢复智能推理的模型绑定。统一问答和智能推理使用同一个大模型，小模型配置不变。

## 运行命令

先校验配置，不发起网络请求：

```powershell
.\tools\qa-regression\run-unified.ps1 -ValidateOnly
.\tools\qa-regression\run-comparison.ps1 -ValidateOnly
```

运行全部启用题目：

```powershell
.\tools\qa-regression\run-unified.ps1
.\tools\qa-regression\run-comparison.ps1
```

按题号运行：

```powershell
.\tools\qa-regression\run-unified.ps1 -QuestionIds Q05,Q07,Q10
```

按标签运行：

```powershell
.\tools\qa-regression\run-comparison.ps1 -Tags compliance,hcp
```

使用另一份题库：

```powershell
.\tools\qa-regression\run-unified.ps1 -QuestionsPath D:\doc\我的回归题库.json
```

## 输出

默认输出到仓库的 `tmp/qa-regression`：

- `统一问答回归-时间-报告.md`
- `统一问答与智能推理对比-时间-报告.md`

每次运行只生成一个中文 Markdown 报告。汇总指标、失败断言、逐题完整回答和原始结构化 JSON 都放在该文件中。每完成一道题都会更新报告，即使后续请求中断，已完成结果仍然保留。

当前对比报告采用可客观计算的指标：耗时、首字时间、引用数、工具调用数、回答长度、路由结果、兜底状态及配置断言。语义正确性仍应结合逐题回答和引用人工复核，避免用回答长度代替回答质量。
