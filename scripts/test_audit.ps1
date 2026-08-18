# =============================================================================
# RocheKAP 审计日志测试脚本
# 用途: 测试登录/登出/文件操作等，并检查 audit_logs 表是否有记录
# 用法: .\scripts\test_audit.ps1 [-AuthPort 8081] [-AppPort 8080]
#       -AuthPort:  Auth Service 端口 (默认 8081)
#       -AppPort:   Application Backend 端口 (默认 8080)
# =============================================================================

param(
    [int]$AuthPort = 8081,
    [int]$AppPort = 8080
)

$ErrorActionPreference = "Stop"
$AuthBase  = "http://localhost:$AuthPort/api/v1"
$AppBase   = "http://localhost:$AppPort/api/v1"

# 测试用户凭据 (会自动注册)
$TEST_EMAIL    = "audit_test_$(Get-Date -Format 'HHmmss')@test.local"
$TEST_PASSWORD = "Test@123456"
$TEST_USERNAME = "audit_tester"

$Token   = $null
$User    = $null

Write-Host "`n========================================"  -ForegroundColor Cyan
Write-Host "   RocheKAP 审计日志功能测试"             -ForegroundColor Cyan
Write-Host "========================================"  -ForegroundColor Cyan
Write-Host "  Auth Service: $AuthBase"                -ForegroundColor Gray
Write-Host "  App Backend:  $AppBase"                 -ForegroundColor Gray
Write-Host "  测试用户:     $TEST_EMAIL"               -ForegroundColor Gray
Write-Host "========================================`n" -ForegroundColor Cyan

# ---- 工具函数 ----
function Invoke-Api {
    param(
        [string]$Method,
        [string]$Url,
        $Body,
        [string]$Token,
        [string]$ContentType = "application/json"
    )
    $headers = @{ "Content-Type" = $ContentType }
    if ($Token) { $headers["Authorization"] = "Bearer $Token" }
    $params = @{
        Method      = $Method
        Uri         = $Url
        Headers     = $headers
        ContentType = $ContentType
    }
    if ($Body) {
        if ($ContentType -eq "application/json") {
            $params["Body"] = ($Body | ConvertTo-Json -Depth 10 -Compress)
        } else {
            $params["Body"] = $Body
        }
    }
    try {
        $response = Invoke-RestMethod @params
        return $response
    } catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        try {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $reader.BaseStream.Position = 0
            $reader.DiscardBufferedData()
            $body = $reader.ReadToEnd()
            return @{ _error = $true; status = $statusCode; body = $body }
        } catch {
            return @{ _error = $true; status = $statusCode; body = $_.Exception.Message }
        }
    }
}

function Write-Step {
    param([string]$Text) 
    Write-Host "`n>>> $Text" -ForegroundColor Yellow
}

function Write-OK {
    param([string]$Text) 
    Write-Host "  [OK] $Text" -ForegroundColor Green
}

function Write-FAIL {
    param([string]$Text) 
    Write-Host "  [FAIL] $Text" -ForegroundColor Red
}

# =============================================================================
# 1. 检查服务是否在运行
# =============================================================================
Write-Step "检查服务状态"

$authOk = $false
$appOk  = $false

try {
    $null = Invoke-RestMethod -Uri "$AuthBase/auth/registration/config" -Method GET -TimeoutSec 3
    $authOk = $true
    Write-OK "Auth Service ($AuthBase) 在线"
} catch {
    Write-FAIL "Auth Service ($AuthBase) 不可达: $_"
}

try {
    # Application Backend 可能需要认证，所以 401 也算在线
    try {
        $null = Invoke-RestMethod -Uri "$AppBase/system/admin/audit-log" -Method GET -TimeoutSec 3
    } catch {
        if ($_.Exception.Response.StatusCode.value__ -eq 401) {
            $appOk = $true
        }
    }
    Write-OK "App Backend ($AppBase) 在线"
} catch {
    Write-FAIL "App Backend ($AppBase) 不可达"
}

if (-not $authOk) {
    Write-Host "`n请先启动服务: bash scripts/server_dev.sh up" -ForegroundColor Red
    exit 1
}
if (-not $appOk) {
    Write-Host "`nApp Backend 不可达，后续查询审计日志将失败" -ForegroundColor Red
}

# =============================================================================
# 2. 注册测试用户
# =============================================================================
Write-Step "注册测试用户"
$regResp = Invoke-Api -Method POST -Url "$AuthBase/auth/register" -Body @{
    username = $TEST_USERNAME
    email    = $TEST_EMAIL
    password = $TEST_PASSWORD
}

if ($regResp._error) {
    if ($regResp.body -match "already exists|已存在|duplicate") {
        Write-OK "用户已存在 (跳过注册): $TEST_EMAIL"
    } else {
        Write-FAIL "注册失败: $($regResp.body)"
        # 继续尝试登录（可能之前已注册过）
    }
} elseif ($regResp.success) {
    Write-OK "注册成功: $TEST_EMAIL"
}

# =============================================================================
# 3. 测试登录成功
# =============================================================================
Write-Step "测试登录 (预期: 成功)"
$loginResp = Invoke-Api -Method POST -Url "$AuthBase/auth/login" -Body @{
    email    = $TEST_EMAIL
    password = $TEST_PASSWORD
}

if ($loginResp.success) {
    $Token = $loginResp.token
    $User  = $loginResp.user
    Write-OK "登录成功! Token: $($Token.Substring(0, 20))..."
    Write-OK "用户ID: $($User.id), 用户名: $($User.username)"
} else {
    Write-FAIL "登录失败: $($loginResp | ConvertTo-Json)"
    exit 1
}

# =============================================================================
# 4. 测试登录失败 (错误密码)
# =============================================================================
Write-Step "测试登录失败 (错误密码，预期: 401)"
$badLogin = Invoke-Api -Method POST -Url "$AuthBase/auth/login" -Body @{
    email    = $TEST_EMAIL
    password = "WrongPassword123"
}
if ($badLogin._error) {
    Write-OK "错误密码登录被拒绝 (符合预期): $($badLogin.status)"
} elseif (-not $badLogin.success) {
    Write-OK "错误密码登录失败 (符合预期)"
} else {
    Write-FAIL "错误密码居然登录成功了? (异常)"
}

# =============================================================================
# 5. 验证Token有效性
# =============================================================================
Write-Step "验证Token与获取当前用户"
$meResp = Invoke-Api -Method GET -Url "$AuthBase/auth/me" -Token $Token
if ($meResp.success -or $meResp.id) {
    Write-OK "Token验证成功: $($meResp.email)"
} else {
    Write-FAIL "Token验证失败"
}

# =============================================================================
# 6. 测试登出
# =============================================================================
Write-Step "测试登出"
$logoutResp = Invoke-Api -Method POST -Url "$AuthBase/auth/logout" -Token $Token
if ($logoutResp.success -or $logoutResp.message) {
    Write-OK "登出成功: $($logoutResp.message)"
} else {
    Write-FAIL "登出失败 (可能是预期行为，有些实现返回空): $($logoutResp | ConvertTo-Json)"
}

# 登出后Token应失效
Write-Step "验证登出后Token失效"
$afterLogout = Invoke-Api -Method GET -Url "$AuthBase/auth/me" -Token $Token
if ($afterLogout._error -and $afterLogout.status -eq 401) {
    Write-OK "登出后Token已失效 (符合预期)"
} else {
    Write-FAIL "登出后Token仍有效 (异常)"
}

# 重新登录以继续后续测试
Write-Step "重新登录 (为后续测试获取新Token)"
$loginResp2 = Invoke-Api -Method POST -Url "$AuthBase/auth/login" -Body @{
    email    = $TEST_EMAIL
    password = $TEST_PASSWORD
}
if ($loginResp2.success) {
    $Token = $loginResp2.token
    Write-OK "重新登录成功"
} else {
    Write-FAIL "重新登录失败!"
    exit 1
}

# =============================================================================
# 7. 文件操作测试 (需先获取知识库ID)
# =============================================================================
Write-Step "查询知识库列表 (用于文件操作)"
$kbList = Invoke-Api -Method GET -Url "$AppBase/knowledge-bases" -Token $Token
$kbId = $null

if ($kbList.data -and $kbList.data.Count -gt 0) {
    $kbId = $kbList.data[0].id
    Write-OK "找到知识库: ID=$kbId, Name=$($kbList.data[0].name)"
} else {
    Write-Host "  [WARN] 没有知识库，跳过文件操作测试" -ForegroundColor DarkYellow
    Write-Host "  可通过API创建知识库: POST $AppBase/knowledge-bases" -ForegroundColor Gray
}

if ($kbId) {
    # 7a. 创建手动知识 (不需要上传文件)
    Write-Step "创建手动知识条目"
    $manualResp = Invoke-Api -Method POST -Url "$AppBase/knowledge-bases/$kbId/knowledge/manual" -Token $Token -Body @{
        title   = "Audit Test Document $(Get-Date -Format 'yyyyMMddHHmmss')"
        content = "这是一个审计日志测试文档。用于验证 knowledge.created 事件是否正确记录到 audit_logs 表。"
        status  = "published"
    }
    if ($manualResp.success -or $manualResp.id) {
        Write-OK "手动知识创建成功: ID=$($manualResp.id)"
        $knowledgeId = $manualResp.id
    } elseif ($manualResp._error) {
        Write-FAIL "创建手动知识失败: status=$($manualResp.status) $($manualResp.body)"
    } else {
        Write-FAIL "创建手动知识失败: $($manualResp | ConvertTo-Json)"
    }

    # 7b. 查询知识列表
    if ($knowledgeId) {
        Write-Step "查询知识详情"
        $knowledgeResp = Invoke-Api -Method GET -Url "$AppBase/knowledge/$knowledgeId" -Token $Token
        if ($knowledgeResp.id) {
            Write-OK "知识详情查询成功: $($knowledgeResp.title)"
        }
        
        # 7c. 下载知识文件
        Write-Step "下载知识文件 (触发下载审计)"
        try {
            $downloadResp = Invoke-RestMethod -Uri "$AppBase/knowledge/$knowledgeId/download" `
                -Method GET -Headers @{ "Authorization" = "Bearer $Token" } `
                -TimeoutSec 5
            Write-OK "下载请求已发起"
        } catch {
            # 手动知识可能没有文件可下载
            Write-Host "  [INFO] 下载请求: $($_.Exception.Message.Substring(0, [Math]::Min(50, $_.Exception.Message.Length)))" -ForegroundColor Gray
        }
    }
}

# =============================================================================
# 8. 查询审计日志
# =============================================================================
Write-Step "========================================="
Write-Step "  查询审计日志 (audit_logs 表)" 
Write-Step "========================================="

Start-Sleep -Seconds 2  # 等待异步写入完成

# 8a. 查询系统级审计日志
Write-Step "查询系统级审计日志 (GET /system/admin/audit-log)"
$sysAudit = Invoke-Api -Method GET -Url "$AppBase/system/admin/audit-log?limit=20" -Token $Token
if ($sysAudit.success) {
    $count = if ($sysAudit.data) { $sysAudit.data.Count } else { 0 }
    Write-OK "系统审计日志: 返回 $count 条记录"

    if ($count -gt 0) {
        Write-Host "`n  --- 系统审计日志摘要 ---" -ForegroundColor Cyan
        $sysAudit.data | ForEach-Object {
            $action  = $_.action
            $outcome = $_.outcome
            $time    = $_.created_at
            $id      = $_.id
            $path    = $_.request_path
            $detail  = if ($_.details) { ($_.details | ConvertTo-Json -Compress).Substring(0, [Math]::Min(40, ($_.details | ConvertTo-Json -Compress).Length)) } else { "" }
            $color   = if ($outcome -eq "denied" -or $outcome -eq "失败") { "Red" } else { "Green" }
            Write-Host "  #${id} [$time] action=$action outcome=$outcome path=$path detail=$detail" -ForegroundColor $color
        }
    } else {
        Write-Host "  [WARN] 系统审计日志为空 — 可能 audit_logs 表仍未写入!" -ForegroundColor Red
    }
} else {
    Write-FAIL "查询系统审计日志失败: $($sysAudit | ConvertTo-Json)"
}

# 8b. 按 action 过滤 - 登录事件
Write-Step "按 action 过滤: 认证.登录 / 认证.登录失败 / 认证.登出"
$loginActions = Invoke-Api -Method GET -Url "$AppBase/system/admin/audit-log?limit=20&action=认证.登录" -Token $Token
if ($loginActions.success -and $loginActions.data) {
    Write-OK "登录审计事件: $($loginActions.data.Count) 条"
    $loginActions.data | ForEach-Object {
        Write-Host "  #$($_.id) [$($_.created_at)] actor=$($_.actor_user_id) outcome=$($_.outcome) detail=$($_.details | ConvertTo-Json -Compress)" -ForegroundColor Green
    }
} else {
    Write-Host "  [WARN] 未找到 认证.登录 审计记录" -ForegroundColor DarkYellow
}

# 8c. 按 action 过滤 - 知识操作
Write-Step "按 action 过滤: knowledge.created"
$knowledgeActions = Invoke-Api -Method GET -Url "$AppBase/system/admin/audit-log?limit=20&action=knowledge.created" -Token $Token
if ($knowledgeActions.success -and $knowledgeActions.data) {
    Write-OK "知识创建审计事件: $($knowledgeActions.data.Count) 条"
    $knowledgeActions.data | ForEach-Object {
        Write-Host "  #$($_.id) [$($_.created_at)] actor=$($_.actor_user_id) outcome=$($_.outcome) detail=$($_.details | ConvertTo-Json -Compress)" -ForegroundColor Green
    }
} else {
    Write-Host "  [WARN] 未找到 knowledge.created 审计记录" -ForegroundColor DarkYellow
}

# 8d. 按 action 过滤 - HTTP请求审计
Write-Step "按 action 过滤: http.request"
$httpActions = Invoke-Api -Method GET -Url "$AppBase/system/admin/audit-log?limit=20&action=http.request" -Token $Token
if ($httpActions.success -and $httpActions.data) {
    Write-OK "HTTP请求审计事件: $($httpActions.data.Count) 条"
    $httpActions.data | ForEach-Object {
        Write-Host "  #$($_.id) [$($_.created_at)] path=$($_.request_path) method=$($_.request_method) outcome=$($_.outcome)" -ForegroundColor Green
    }
} else {
    Write-Host "  [WARN] 未找到 http.request 审计记录" -ForegroundColor DarkYellow
}

# =============================================================================
# 9. 知识域审计日志查询
# =============================================================================
Write-Step "查询知识域列表 (用于域级审计日志)"
$domainList = Invoke-Api -Method GET -Url "$AppBase/knowledge-domains" -Token $Token
if ($domainList.data -and $domainList.data.Count -gt 0) {
    $domainId = $domainList.data[0].id
    Write-OK "知识域: ID=$domainId"
    
    Write-Step "查询域级审计日志 (GET /knowledge-domains/$domainId/audit-log)"
    $domainAudit = Invoke-Api -Method GET -Url "$AppBase/knowledge-domains/$domainId/audit-log?limit=10" -Token $Token
    if ($domainAudit.success) {
        $dcount = if ($domainAudit.data) { $domainAudit.data.Count } else { 0 }
        Write-OK "域审计日志: $dcount 条"
        if ($dcount -gt 0) {
            $domainAudit.data | ForEach-Object {
                Write-Host "  #$($_.id) [$($_.created_at)] action=$($_.action) outcome=$($_.outcome)" -ForegroundColor Gray
            }
        }
    }
}

# =============================================================================
# 10. 直接查询数据库 (如果docker可用)
# =============================================================================
Write-Step "直接查询数据库 audit_logs 表"
Write-Host "  表名: audit_logs" -ForegroundColor Gray

# 尝试通过 docker exec 直接查 PostgreSQL
$dbCheckCmd = "docker exec -it $(docker ps --filter 'name=postgres' --format '{{.Names}}' | Select-Object -First 1) psql -U postgres -d rochekap -c 'SELECT count(*) as total FROM audit_logs; SELECT id, action, outcome, actor_user_id, created_at FROM audit_logs ORDER BY id DESC LIMIT 10;' 2>&1"
Write-Host "  可手动执行以下命令查看数据库:`n" -ForegroundColor Gray
Write-Host "  $dbCheckCmd`n" -ForegroundColor Cyan

# =============================================================================
# 11. 总结
# =============================================================================
Write-Step "========================================="
Write-Step "  测试完成！总结"
Write-Step "========================================="
Write-Host "  如果 audit_logs 返回数据 → 修复生效! ID DEFAULT 问题已解决" -ForegroundColor Green
Write-Host "  如果 audit_logs 仍然为空  → 请检查:" -ForegroundColor Red
Write-Host "    1. docker logs 中是否有 audit 相关 ERROR 日志" -ForegroundColor Gray
Write-Host "    2. 迁移 0011 是否已执行: docker exec postgres psql -U postgres -d rochekap -c 'SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 5;'" -ForegroundColor Gray
Write-Host "    3. 序列状态: docker exec postgres psql -U postgres -d rochekap -c \"SELECT last_value FROM audit_logs_id_seq;\"" -ForegroundColor Gray
Write-Host "    4. 直接插入测试: docker exec postgres psql -U postgres -d rochekap -c \"INSERT INTO audit_logs(action, outcome, knowledge_domain_id) VALUES('test.write','success',0) RETURNING id;\"" -ForegroundColor Gray
Write-Host ""
