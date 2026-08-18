$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Net.Http

function Get-EnvironmentValue([string]$name) {
    if ([string]::IsNullOrWhiteSpace($name)) { return "" }
    return [Environment]::GetEnvironmentVariable($name)
}

function Resolve-RegressionModelId($modelConfig, [string]$fallbackModelId = "") {
    $environmentModelId = Get-EnvironmentValue ([string]$modelConfig.model_id_env)
    if (-not [string]::IsNullOrWhiteSpace($environmentModelId)) {
        return $environmentModelId
    }
    $configuredModelId = [string]$modelConfig.model_id
    if (-not [string]::IsNullOrWhiteSpace($configuredModelId)) {
        return $configuredModelId
    }
    return $fallbackModelId
}

function Resolve-RegressionPath([string]$path, [string]$baseDirectory) {
    if ([string]::IsNullOrWhiteSpace($path)) { return "" }
    if ([IO.Path]::IsPathRooted($path)) { return [IO.Path]::GetFullPath($path) }
    return [IO.Path]::GetFullPath((Join-Path $baseDirectory $path))
}

function Read-RegressionJson([string]$path) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Configuration file does not exist: $path"
    }
    try {
        return Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json
    } catch {
        throw "Invalid JSON file ${path}: $($_.Exception.Message)"
    }
}

function Test-RegressionConfiguration($config, [object[]]$questions) {
    if ($null -eq $config -or [int]$config.version -ne 1) {
        throw "config.version must be 1"
    }
    if ([string]::IsNullOrWhiteSpace([string]$config.base_url)) {
        throw "config.base_url is required"
    }
    if ($questions.Count -eq 0) {
        throw "The question file must contain at least one question"
    }
    $ids = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($question in $questions) {
        if ([string]::IsNullOrWhiteSpace([string]$question.id)) {
            throw "Every question requires a non-empty id"
        }
        if (-not $ids.Add([string]$question.id)) {
            throw "Duplicate question id: $($question.id)"
        }
        if ([string]::IsNullOrWhiteSpace([string]$question.question)) {
            throw "Question $($question.id) has empty question text"
        }
    }
}

function Select-RegressionQuestions(
    [object[]]$questions,
    [string[]]$questionIds,
    [string[]]$tags
) {
    $selected = @($questions | Where-Object { $null -eq $_.enabled -or [bool]$_.enabled })
    if ($questionIds.Count -gt 0) {
        $wantedIds = @($questionIds | ForEach-Object { $_.Trim().ToLowerInvariant() })
        $selected = @($selected | Where-Object { ([string]$_.id).ToLowerInvariant() -in $wantedIds })
    }
    if ($tags.Count -gt 0) {
        $wantedTags = @($tags | ForEach-Object { $_.Trim().ToLowerInvariant() })
        $selected = @($selected | Where-Object {
            $questionTags = @($_.tags | ForEach-Object { ([string]$_).ToLowerInvariant() })
            @($wantedTags | Where-Object { $_ -in $questionTags }).Count -gt 0
        })
    }
    if ($selected.Count -eq 0) {
        throw "No enabled questions matched the supplied filters"
    }
    return $selected
}

function ConvertTo-Base64Url([byte[]]$bytes) {
    return [Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
}

function New-RegressionJwt([string]$userId, [string]$email, [string]$secret, [int]$ttlHours) {
    $now = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    $header = '{"alg":"HS256","typ":"JWT"}'
    $payload = [ordered]@{
        user_id = $userId
        email = $email
        is_system_admin = $true
        role_knowledge_officer = 0
        exp = $now + ($ttlHours * 3600)
        iat = $now
        type = "access"
    } | ConvertTo-Json -Compress
    $headerPart = ConvertTo-Base64Url ([Text.Encoding]::UTF8.GetBytes($header))
    $payloadPart = ConvertTo-Base64Url ([Text.Encoding]::UTF8.GetBytes($payload))
    $unsigned = "$headerPart.$payloadPart"
    $hmac = [Security.Cryptography.HMACSHA256]::new([Text.Encoding]::UTF8.GetBytes($secret))
    try {
        $signature = ConvertTo-Base64Url ($hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($unsigned)))
    } finally {
        $hmac.Dispose()
    }
    return "$unsigned.$signature"
}

function ConvertTo-SqlLiteral([string]$value) {
    return $value.Replace("'", "''")
}

function Invoke-RegressionSql($config, [string]$sql) {
    $dbPassword = Get-EnvironmentValue ([string]$config.database.password_env)
    $dbHost = Get-EnvironmentValue ([string]$config.database.host_env)
    if ([string]::IsNullOrWhiteSpace($dbPassword) -or [string]::IsNullOrWhiteSpace($dbHost)) {
        throw "Database-backed test authentication requires $($config.database.password_env) and $($config.database.host_env)"
    }
    $container = [string]$config.database.container
    $port = [int]$config.database.port
    $user = [string]$config.database.user
    $database = [string]$config.database.name
    $output = & docker exec -e "PGPASSWORD=$dbPassword" $container `
        psql -h $dbHost -p $port -U $user -d $database -v ON_ERROR_STOP=1 -P pager=off -At -c $sql
    if ($LASTEXITCODE -ne 0) {
        throw "Regression SQL command failed"
    }
    return $output
}

function Initialize-RegressionAuth($config) {
    $configuredToken = Get-EnvironmentValue ([string]$config.auth.access_token_env)
    if (-not [string]::IsNullOrWhiteSpace($configuredToken)) {
        return [pscustomobject]@{ Token = $configuredToken; TokenId = ""; DatabaseTokenCreated = $false }
    }

    $userId = Get-EnvironmentValue ([string]$config.auth.user_id_env)
    $jwtSecret = Get-EnvironmentValue ([string]$config.auth.jwt_secret_env)
    if ([string]::IsNullOrWhiteSpace($userId) -or [string]::IsNullOrWhiteSpace($jwtSecret)) {
        throw "Set $($config.auth.access_token_env), or configure $($config.auth.user_id_env) and $($config.auth.jwt_secret_env) for temporary test authentication"
    }
    $ttlHours = [int]$config.auth.token_ttl_hours
    if ($ttlHours -lt 1) { $ttlHours = 6 }
    $token = New-RegressionJwt $userId ([string]$config.auth.user_email) $jwtSecret $ttlHours
    $tokenId = [guid]::NewGuid().ToString()
    $escapedToken = ConvertTo-SqlLiteral $token
    $escapedTokenId = ConvertTo-SqlLiteral $tokenId
    $escapedUserId = ConvertTo-SqlLiteral $userId
    $sql = "INSERT INTO auth_tokens (id,user_id,token,token_type,expires_at,is_revoked,created_at,updated_at) VALUES ('$escapedTokenId','$escapedUserId','$escapedToken','access_token',NOW()+INTERVAL '$ttlHours hours',false,NOW(),NOW());"
    $null = Invoke-RegressionSql $config $sql
    return [pscustomobject]@{ Token = $token; TokenId = $tokenId; DatabaseTokenCreated = $true }
}

function Remove-RegressionAuth($config, $auth) {
    if ($null -eq $auth -or -not $auth.DatabaseTokenCreated -or [string]::IsNullOrWhiteSpace([string]$auth.TokenId)) {
        return
    }
    $escapedTokenId = ConvertTo-SqlLiteral ([string]$auth.TokenId)
    $null = Invoke-RegressionSql $config "DELETE FROM auth_tokens WHERE id='$escapedTokenId';"
}

function Initialize-SmartModelBinding($config, [string]$mode, [string]$unifiedModelId) {
    if ($mode -ne "comparison") {
        return $null
    }
    $fallbackModelId = if ([bool]$config.smart_reasoning.use_unified_model_if_empty) { $unifiedModelId } else { "" }
    $targetModelId = Resolve-RegressionModelId $config.smart_reasoning $fallbackModelId
    if ([string]::IsNullOrWhiteSpace($targetModelId)) {
        throw "Comparison mode requires smart_reasoning.model_id, $($config.smart_reasoning.model_id_env), or use_unified_model_if_empty=true"
    }
    if (-not [bool]$config.smart_reasoning.temporarily_bind_model) {
        return [pscustomobject]@{ AgentId = [string]$config.smart_reasoning.agent_id; OriginalModelId = $targetModelId; TargetModelId = $targetModelId; Changed = $false }
    }
    $escapedModelId = ConvertTo-SqlLiteral $targetModelId
    $modelCount = [int]((Invoke-RegressionSql $config "SELECT COUNT(*) FROM models WHERE id='$escapedModelId' AND deleted_at IS NULL AND status='active';" | Select-Object -First 1))
    if ($modelCount -ne 1) {
        throw "Smart-reasoning comparison model is not active: $targetModelId"
    }
    $agentId = [string]$config.smart_reasoning.agent_id
    $escapedAgentId = ConvertTo-SqlLiteral $agentId
    $originalModelId = [string]((Invoke-RegressionSql $config "SELECT COALESCE(config->>'model_id','') FROM custom_agents WHERE id='$escapedAgentId' AND deleted_at IS NULL;" | Select-Object -First 1))
    if ($originalModelId -eq $targetModelId) {
        return [pscustomobject]@{ AgentId = $agentId; OriginalModelId = $originalModelId; TargetModelId = $targetModelId; Changed = $false }
    }
    $null = Invoke-RegressionSql $config "UPDATE custom_agents SET config=jsonb_set(config,'{model_id}',to_jsonb('$escapedModelId'::text),true),updated_at=NOW() WHERE id='$escapedAgentId' AND deleted_at IS NULL;"
    return [pscustomobject]@{ AgentId = $agentId; OriginalModelId = $originalModelId; TargetModelId = $targetModelId; Changed = $true }
}

function Restore-SmartModelBinding($config, $binding) {
    if ($null -eq $binding -or -not $binding.Changed) { return }
    $escapedAgentId = ConvertTo-SqlLiteral ([string]$binding.AgentId)
    $originalModelId = [string]$binding.OriginalModelId
    if ([string]::IsNullOrWhiteSpace($originalModelId)) {
        $null = Invoke-RegressionSql $config "UPDATE custom_agents SET config=config-'model_id',updated_at=NOW() WHERE id='$escapedAgentId' AND deleted_at IS NULL;"
        return
    }
    $escapedOriginalModelId = ConvertTo-SqlLiteral $originalModelId
    $null = Invoke-RegressionSql $config "UPDATE custom_agents SET config=jsonb_set(config,'{model_id}',to_jsonb('$escapedOriginalModelId'::text),true),updated_at=NOW() WHERE id='$escapedAgentId' AND deleted_at IS NULL;"
}

function New-RegressionJsonContent($body) {
    $json = $body | ConvertTo-Json -Depth 30 -Compress
    return [Net.Http.StringContent]::new($json, [Text.Encoding]::UTF8, "application/json")
}

function New-RegressionSession($client, [string]$baseUrl, [string]$title) {
    $content = New-RegressionJsonContent ([ordered]@{ title = $title; description = "qa regression" })
    try {
        $response = $client.PostAsync("$baseUrl/api/v1/sessions", $content).GetAwaiter().GetResult()
        $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) {
            throw "Create session failed: HTTP $([int]$response.StatusCode): $text"
        }
        return [string](($text | ConvertFrom-Json).data.id)
    } finally {
        $content.Dispose()
    }
}

function Get-RegressionFinalMessage($client, [string]$baseUrl, [string]$sessionId) {
    for ($attempt = 1; $attempt -le 10; $attempt++) {
        $response = $client.GetAsync("$baseUrl/api/v1/messages/$sessionId/load?limit=20").GetAwaiter().GetResult()
        $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        if ($response.IsSuccessStatusCode) {
            $messages = @(($text | ConvertFrom-Json).data)
            $assistant = @($messages |
                Where-Object { $_.role -eq "assistant" } |
                Sort-Object created_at -Descending |
                Select-Object -First 1)
            if ($assistant.Count -gt 0 -and [bool]$assistant[0].is_completed) {
                return $assistant[0]
            }
        }
        Start-Sleep -Milliseconds 500
    }
    return $null
}

function Get-RegressionObservation($client, [string]$baseUrl, [string]$runId) {
    if ([string]::IsNullOrWhiteSpace($runId)) { return $null }
    $response = $client.GetAsync("$baseUrl/api/v1/unified-qa/runs/$runId").GetAwaiter().GetResult()
    $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    if (-not $response.IsSuccessStatusCode) {
        return [pscustomobject]@{ http_status = [int]$response.StatusCode; error = $text }
    }
    return ($text | ConvertFrom-Json).data
}

function Invoke-RegressionChat(
    $client,
    $config,
    [string]$mode,
    [string]$questionId,
    [string]$question,
    [string]$sessionId,
    [string]$unifiedModelId
) {
    $baseUrl = ([string]$config.base_url).TrimEnd('/')
    if ($mode -eq "unified") {
        $path = "/api/v1/knowledge-chat/$sessionId"
        $body = [ordered]@{
            query = $question
            agent_enabled = $false
            summary_model_id = $unifiedModelId
            enable_memory = [bool]$config.unified.enable_memory
            disable_title = [bool]$config.unified.disable_title
            channel = [string]$config.channel
        }
    } else {
        $path = "/api/v1/agent-chat/$sessionId"
        $body = [ordered]@{
            query = $question
            agent_enabled = $true
            agent_id = [string]$config.smart_reasoning.agent_id
            enable_memory = [bool]$config.smart_reasoning.enable_memory
            disable_title = [bool]$config.smart_reasoning.disable_title
            web_search_enabled = [bool]$config.smart_reasoning.web_search_enabled
            channel = [string]$config.channel
        }
    }

    $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Post, "$baseUrl$path")
    $request.Headers.Add("X-Request-ID", "qa-regression-$questionId-$mode-$([guid]::NewGuid().ToString('N').Substring(0, 10))")
    $request.Content = New-RegressionJsonContent $body
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $events = [Collections.Generic.List[object]]::new()
    $answerParts = [Collections.Generic.List[string]]::new()
    $runId = ""
    $firstAnswerMs = $null
    $httpStatus = 0
    $errorText = ""
    try {
        $response = $client.SendAsync($request, [Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
        $httpStatus = [int]$response.StatusCode
        if (-not $response.IsSuccessStatusCode) {
            $errorText = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        } else {
            $stream = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
            $reader = [IO.StreamReader]::new($stream, [Text.Encoding]::UTF8)
            try {
                $complete = $false
                $heartbeatSeconds = [int]$config.heartbeat_seconds
                if ($heartbeatSeconds -lt 5) { $heartbeatSeconds = 30 }
                while (-not $reader.EndOfStream -and -not $complete) {
                    $lineTask = $reader.ReadLineAsync()
                    while (-not $lineTask.Wait($heartbeatSeconds * 1000)) {
                        Write-Output ("HEARTBEAT id={0} mode={1} elapsed_s={2}" -f $questionId, $mode, [int]$watch.Elapsed.TotalSeconds)
                    }
                    $line = $lineTask.GetAwaiter().GetResult()
                    if ($null -eq $line -or -not $line.StartsWith("data:")) { continue }
                    $json = $line.Substring(5).Trim()
                    if ([string]::IsNullOrWhiteSpace($json)) { continue }
                    try {
                        $event = $json | ConvertFrom-Json
                    } catch {
                        $events.Add([pscustomobject]@{ parse_error = $_.Exception.Message; raw = $json })
                        continue
                    }
                    $events.Add($event)
                    if (-not [string]::IsNullOrWhiteSpace([string]$event.data.run_id)) {
                        $runId = [string]$event.data.run_id
                    }
                    if ($event.response_type -eq "answer" -and -not [string]::IsNullOrEmpty([string]$event.content)) {
                        if ($null -eq $firstAnswerMs) { $firstAnswerMs = [int]$watch.ElapsedMilliseconds }
                        $answerParts.Add([string]$event.content)
                    }
                    if ($event.response_type -in @("question_understood", "knowledge_retrieved", "answer", "complete")) {
                        Write-Output ("EVENT id={0} mode={1} type={2} elapsed_s={3}" -f $questionId, $mode, $event.response_type, [int]$watch.Elapsed.TotalSeconds)
                    }
                    if ($event.response_type -eq "complete") { $complete = $true }
                }
            } finally {
                $reader.Dispose()
                $stream.Dispose()
            }
        }
        $watch.Stop()
        $finalMessage = Get-RegressionFinalMessage $client $baseUrl $sessionId
        if ($answerParts.Count -eq 0 -and $null -ne $finalMessage -and -not [string]::IsNullOrWhiteSpace([string]$finalMessage.content)) {
            $answerParts.Add([string]$finalMessage.content)
        }
        $observation = $null
        if ($mode -eq "unified") {
            $observation = Get-RegressionObservation $client $baseUrl $runId
        }
        return [pscustomobject]@{
            mode = $mode
            session_id = $sessionId
            run_id = $runId
            http_status = $httpStatus
            elapsed_ms = [int]$watch.ElapsedMilliseconds
            first_answer_ms = $firstAnswerMs
            answer = ($answerParts -join "")
            error = $errorText
            events = $events
            final_message = $finalMessage
            unified_observation = $observation
        }
    } catch {
        $watch.Stop()
        return [pscustomobject]@{
            mode = $mode
            session_id = $sessionId
            run_id = $runId
            http_status = $httpStatus
            elapsed_ms = [int]$watch.ElapsedMilliseconds
            first_answer_ms = $firstAnswerMs
            answer = ($answerParts -join "")
            error = $_.Exception.ToString()
            events = $events
            final_message = $null
            unified_observation = $null
        }
    } finally {
        $request.Dispose()
    }
}

function Get-UnifiedRun($result) {
    if ($null -eq $result -or $null -eq $result.unified_observation) { return $null }
    if ($null -ne $result.unified_observation.run) { return $result.unified_observation.run }
    return $result.unified_observation
}

function Get-RegressionFallbackFlag($result) {
    if ($null -eq $result) { return $false }
    foreach ($event in @($result.events)) {
        if ($event.response_type -eq "answer" -and ($event.is_fallback -eq $true -or $event.data.is_fallback -eq $true)) {
            return $true
        }
    }
    return $false
}

function Get-RegressionSummary($pair) {
    $run = Get-UnifiedRun $pair.unified
    $metrics = if ($null -ne $run) { $run.metrics } else { $null }
    $selectedAgents = if ($null -ne $run) { @($run.selected_agent_ids) } else { @() }
    return [pscustomobject]@{
        id = [string]$pair.id
        question = [string]$pair.question
        unified_http_status = if ($null -ne $pair.unified) { [int]$pair.unified.http_status } else { 0 }
        unified_elapsed_ms = if ($null -ne $pair.unified) { [int]$pair.unified.elapsed_ms } else { 0 }
        unified_first_answer_ms = if ($null -ne $pair.unified) { $pair.unified.first_answer_ms } else { $null }
        unified_status = if ($null -ne $run) { [string]$run.status } else { "" }
        unified_selected_agent_ids = $selectedAgents
        unified_route_outcome = if ($null -ne $metrics) { [string]$metrics.route_outcome } else { "" }
        unified_reference_count = if ($null -ne $metrics -and $null -ne $metrics.reference_count) { [int]$metrics.reference_count } else { 0 }
        unified_tool_calls = if ($null -ne $metrics -and $null -ne $metrics.tool_calls) { [int]$metrics.tool_calls } else { 0 }
        unified_citation_recovery = if ($null -ne $metrics) { [bool]$metrics.citation_validation_failed } else { $false }
        unified_is_fallback = Get-RegressionFallbackFlag $pair.unified
        unified_answer_length = if ($null -ne $pair.unified) { ([string]$pair.unified.answer).Length } else { 0 }
        smart_http_status = if ($null -ne $pair.smart_reasoning) { [int]$pair.smart_reasoning.http_status } else { 0 }
        smart_elapsed_ms = if ($null -ne $pair.smart_reasoning) { [int]$pair.smart_reasoning.elapsed_ms } else { 0 }
        smart_first_answer_ms = if ($null -ne $pair.smart_reasoning) { $pair.smart_reasoning.first_answer_ms } else { $null }
        smart_answer_length = if ($null -ne $pair.smart_reasoning) { ([string]$pair.smart_reasoning.answer).Length } else { 0 }
    }
}

function Add-RegressionCheck($checks, [string]$name, [bool]$passed, [string]$expected, [string]$actual) {
    $checks.Add([pscustomobject]@{ name = $name; passed = $passed; expected = $expected; actual = $actual })
}

function Test-RegressionExpectations($question, $pair, $summary, $config) {
    $checks = [Collections.Generic.List[object]]::new()
    Add-RegressionCheck $checks "unified.http_status" ($summary.unified_http_status -eq 200) "200" ([string]$summary.unified_http_status)
    Add-RegressionCheck $checks "unified.answer_not_empty" ($summary.unified_answer_length -gt 0) "> 0" ([string]$summary.unified_answer_length)
    $maxLatency = [int]$config.assertions.default_max_latency_ms
    if ($null -ne $question.expected.unified.max_latency_ms) {
        $maxLatency = [int]$question.expected.unified.max_latency_ms
    }
    if ($maxLatency -gt 0) {
        Add-RegressionCheck $checks "unified.max_latency_ms" ($summary.unified_elapsed_ms -le $maxLatency) "<= $maxLatency" ([string]$summary.unified_elapsed_ms)
    }

    $expectedAgents = @($question.expected.route.selected_agent_ids | ForEach-Object { [string]$_ } | Sort-Object)
    if ($null -ne $question.expected.route -and $null -ne $question.expected.route.selected_agent_ids) {
        $actualAgents = @($summary.unified_selected_agent_ids | ForEach-Object { [string]$_ } | Sort-Object)
        $sameAgents = (@(Compare-Object $expectedAgents $actualAgents).Count -eq 0)
        Add-RegressionCheck $checks "route.selected_agent_ids" $sameAgents ($expectedAgents -join ",") ($actualAgents -join ",")
    }
    if (-not [string]::IsNullOrWhiteSpace([string]$question.expected.route.outcome)) {
        $expectedOutcome = [string]$question.expected.route.outcome
        Add-RegressionCheck $checks "route.outcome" ($summary.unified_route_outcome -eq $expectedOutcome) $expectedOutcome $summary.unified_route_outcome
    }
    $allowedStatus = @($question.expected.unified.allowed_status | ForEach-Object { [string]$_ })
    if ($allowedStatus.Count -gt 0) {
        Add-RegressionCheck $checks "unified.status" ($summary.unified_status -in $allowedStatus) ($allowedStatus -join ",") $summary.unified_status
    }
    if ($null -ne $question.expected.unified.min_references) {
        $minReferences = [int]$question.expected.unified.min_references
        Add-RegressionCheck $checks "unified.min_references" ($summary.unified_reference_count -ge $minReferences) ">= $minReferences" ([string]$summary.unified_reference_count)
    }
    if ($null -ne $question.expected.unified.is_fallback) {
        $expectedFallback = [bool]$question.expected.unified.is_fallback
        Add-RegressionCheck $checks "unified.is_fallback" ($summary.unified_is_fallback -eq $expectedFallback) ([string]$expectedFallback) ([string]$summary.unified_is_fallback)
    }
    $answer = if ($null -ne $pair.unified) { [string]$pair.unified.answer } else { "" }
    foreach ($required in @($question.expected.unified.must_contain)) {
        $requiredText = [string]$required
        if ([string]::IsNullOrWhiteSpace($requiredText)) { continue }
        Add-RegressionCheck $checks "unified.must_contain" ($answer.IndexOf($requiredText, [StringComparison]::OrdinalIgnoreCase) -ge 0) $requiredText "answer"
    }
    foreach ($forbidden in @($question.expected.unified.must_not_contain)) {
        $forbiddenText = [string]$forbidden
        if ([string]::IsNullOrWhiteSpace($forbiddenText)) { continue }
        Add-RegressionCheck $checks "unified.must_not_contain" ($answer.IndexOf($forbiddenText, [StringComparison]::OrdinalIgnoreCase) -lt 0) "not: $forbiddenText" "answer"
    }
    if ($null -ne $pair.smart_reasoning) {
        Add-RegressionCheck $checks "smart.http_status" ($summary.smart_http_status -eq 200) "200" ([string]$summary.smart_http_status)
        Add-RegressionCheck $checks "smart.answer_not_empty" ($summary.smart_answer_length -gt 0) "> 0" ([string]$summary.smart_answer_length)
    }
    return $checks
}

function ConvertTo-MarkdownCell([string]$value) {
    if ($null -eq $value) { return "" }
    return $value.Replace("|", "\|").Replace("`r", " ").Replace("`n", " ")
}

function Get-AverageMilliseconds([object[]]$values) {
    $numbers = @($values | Where-Object { $null -ne $_ -and [double]$_ -gt 0 } | ForEach-Object { [double]$_ })
    if ($numbers.Count -eq 0) { return 0 }
    return [math]::Round(($numbers | Measure-Object -Average).Average)
}

function New-RegressionMarkdownReport([string]$mode, $runData, [bool]$includeFullAnswers) {
    $builder = [Text.StringBuilder]::new()
    $title = if ($mode -eq "unified") { "统一问答链路回归报告" } else { "统一问答与智能推理对比报告" }
    $null = $builder.AppendLine("# $title")
    $null = $builder.AppendLine()
    $null = $builder.AppendLine("- 开始时间：$($runData.metadata.started_at)")
    $null = $builder.AppendLine("- 完成时间：$($runData.completed_at)")
    $runState = if ([string]$runData.metadata.run_state -eq "completed") { "已完成" } else { "运行中（已保存完成题目）" }
    $null = $builder.AppendLine("- 运行状态：$runState")
    $null = $builder.AppendLine("- 题目数量：$($runData.results.Count)")
    $null = $builder.AppendLine("- 断言：通过 $($runData.assertion_summary.passed)，失败 $($runData.assertion_summary.failed)")
    $null = $builder.AppendLine()

    $unifiedAverage = Get-AverageMilliseconds @($runData.results | ForEach-Object { $_.summary.unified_elapsed_ms })
    $null = $builder.AppendLine("## 汇总")
    $null = $builder.AppendLine()
    $null = $builder.AppendLine("统一问答平均耗时：$unifiedAverage ms。")
    if ($mode -eq "comparison") {
        $smartAverage = Get-AverageMilliseconds @($runData.results | ForEach-Object { $_.summary.smart_elapsed_ms })
        $null = $builder.AppendLine("智能推理平均耗时：$smartAverage ms。")
    }
    $null = $builder.AppendLine()

    if ($mode -eq "unified") {
        $null = $builder.AppendLine("| 题号 | 状态 | 路由 | 引用 | 工具调用 | 兜底 | 引用恢复 | 耗时 | 断言 |")
        $null = $builder.AppendLine("|---|---|---|---:|---:|---|---|---:|---|")
        foreach ($item in $runData.results) {
            $summary = $item.summary
            $assertion = if (@($item.checks | Where-Object { -not $_.passed }).Count -eq 0) { "通过" } else { "失败" }
            $route = $summary.unified_selected_agent_ids -join ','
            if ([string]::IsNullOrWhiteSpace($route)) { $route = $summary.unified_route_outcome }
            $null = $builder.AppendLine("| $($item.id) | $($summary.unified_status) | $(ConvertTo-MarkdownCell $route) | $($summary.unified_reference_count) | $($summary.unified_tool_calls) | $($summary.unified_is_fallback) | $($summary.unified_citation_recovery) | $($summary.unified_elapsed_ms) | $assertion |")
        }
    } else {
        $null = $builder.AppendLine("| 题号 | 统一状态 | 引用 | 统一耗时 | 智能推理耗时 | 耗时差 | 统一长度 | 智能长度 | 断言 |")
        $null = $builder.AppendLine("|---|---|---:|---:|---:|---:|---:|---:|---|")
        foreach ($item in $runData.results) {
            $summary = $item.summary
            $assertion = if (@($item.checks | Where-Object { -not $_.passed }).Count -eq 0) { "通过" } else { "失败" }
            $delta = $summary.unified_elapsed_ms - $summary.smart_elapsed_ms
            $null = $builder.AppendLine("| $($item.id) | $($summary.unified_status) | $($summary.unified_reference_count) | $($summary.unified_elapsed_ms) | $($summary.smart_elapsed_ms) | $delta | $($summary.unified_answer_length) | $($summary.smart_answer_length) | $assertion |")
        }
    }
    $null = $builder.AppendLine()

    $failedItems = @($runData.results | Where-Object { @($_.checks | Where-Object { -not $_.passed }).Count -gt 0 })
    if ($failedItems.Count -gt 0) {
        $null = $builder.AppendLine("## 失败断言")
        $null = $builder.AppendLine()
        foreach ($item in $failedItems) {
            foreach ($check in @($item.checks | Where-Object { -not $_.passed })) {
                $null = $builder.AppendLine("- $($item.id) $($check.name)：期望 [$($check.expected)]，实际 [$($check.actual)]")
            }
        }
        $null = $builder.AppendLine()
    }

    if ($includeFullAnswers) {
        $null = $builder.AppendLine("## 逐题回答")
        $null = $builder.AppendLine()
        foreach ($item in $runData.results) {
            $null = $builder.AppendLine("### $($item.id) $($item.question)")
            $null = $builder.AppendLine()
            $null = $builder.AppendLine("#### 统一问答")
            $null = $builder.AppendLine()
            $null = $builder.AppendLine([string]$item.unified.answer)
            $null = $builder.AppendLine()
            if ($mode -eq "comparison") {
                $null = $builder.AppendLine("#### 智能推理")
                $null = $builder.AppendLine()
                $null = $builder.AppendLine([string]$item.smart_reasoning.answer)
                $null = $builder.AppendLine()
            }
        }
    }

    $null = $builder.AppendLine("## 原始结构化数据")
    $null = $builder.AppendLine()
    $null = $builder.AppendLine("以下 JSON 与本报告的汇总和回答来自同一次运行，便于后续自动分析。")
    $null = $builder.AppendLine()
    $null = $builder.AppendLine('```json')
    $null = $builder.AppendLine(($runData | ConvertTo-Json -Depth 100))
    $null = $builder.AppendLine('```')
    return $builder.ToString()
}

function Write-RegressionMarkdownReport([string]$path, [string]$mode, $runData, [bool]$includeFullAnswers) {
    $report = New-RegressionMarkdownReport $mode $runData $includeFullAnswers
    [IO.File]::WriteAllText($path, $report, [Text.UTF8Encoding]::new($false))
}

function Invoke-QARegression {
    param(
        [ValidateSet("unified", "comparison")]
        [string]$Mode,
        [string]$ConfigPath,
        [string]$QuestionsPath = "",
        [string[]]$QuestionIds = @(),
        [string[]]$Tags = @(),
        [switch]$ValidateOnly
    )

    $resolvedConfigPath = [IO.Path]::GetFullPath($ConfigPath)
    $configDirectory = Split-Path -Parent $resolvedConfigPath
    $config = Read-RegressionJson $resolvedConfigPath
    if ([string]::IsNullOrWhiteSpace($QuestionsPath)) {
        $QuestionsPath = [string]$config.questions_file
    }
    $resolvedQuestionsPath = Resolve-RegressionPath $QuestionsPath $configDirectory
    $questions = @(Read-RegressionJson $resolvedQuestionsPath)
    Test-RegressionConfiguration $config $questions
    $selectedQuestions = @(Select-RegressionQuestions $questions $QuestionIds $Tags)

    if ($ValidateOnly) {
        Write-Output "VALID configuration=$resolvedConfigPath questions=$resolvedQuestionsPath selected=$($selectedQuestions.Count) mode=$Mode"
        return
    }

    $unifiedModelId = Resolve-RegressionModelId $config.unified
    if ([string]::IsNullOrWhiteSpace($unifiedModelId)) {
        throw "Configure unified.model_id or environment variable $($config.unified.model_id_env)"
    }

    $repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
    $outputDirectory = Resolve-RegressionPath ([string]$config.output_directory) $repoRoot
    $null = New-Item -ItemType Directory -Path $outputDirectory -Force
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $prefix = if ($Mode -eq "unified") { "统一问答回归" } else { "统一问答与智能推理对比" }
    $reportPath = Join-Path $outputDirectory "$prefix-$stamp-报告.md"

    $auth = $null
    $smartBinding = $null
    $handler = $null
    $client = $null
    $results = [Collections.Generic.List[object]]::new()
    $startedAt = (Get-Date).ToString("o")
    try {
        $auth = Initialize-RegressionAuth $config
        $smartBinding = Initialize-SmartModelBinding $config $Mode $unifiedModelId
        $handler = [Net.Http.HttpClientHandler]::new()
        $handler.AutomaticDecompression = [Net.DecompressionMethods]::GZip -bor [Net.DecompressionMethods]::Deflate
        $client = [Net.Http.HttpClient]::new($handler)
        $client.Timeout = [Threading.Timeout]::InfiniteTimeSpan
        $client.DefaultRequestHeaders.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", [string]$auth.Token)
        $client.DefaultRequestHeaders.Accept.Add([Net.Http.Headers.MediaTypeWithQualityHeaderValue]::new("text/event-stream"))

        foreach ($question in $selectedQuestions) {
            $questionId = [string]$question.id
            $questionText = [string]$question.question
            Write-Output "QUESTION_START id=$questionId mode=$Mode question=$questionText"
            $pair = [pscustomobject]@{
                id = $questionId
                tags = @($question.tags)
                question = $questionText
                unified = $null
                smart_reasoning = $null
                summary = $null
                checks = @()
            }
            $unifiedSession = New-RegressionSession $client ([string]$config.base_url).TrimEnd('/') "[qa-regression][$questionId][unified]"
            $pair.unified = Invoke-RegressionChat $client $config "unified" $questionId $questionText $unifiedSession $unifiedModelId
            Write-Output ("MODE_DONE id={0} mode=unified http={1} elapsed_s={2}" -f $questionId, $pair.unified.http_status, [math]::Round($pair.unified.elapsed_ms / 1000.0, 3))

            if ($Mode -eq "comparison") {
                $smartSession = New-RegressionSession $client ([string]$config.base_url).TrimEnd('/') "[qa-regression][$questionId][smart]"
                $pair.smart_reasoning = Invoke-RegressionChat $client $config "smart" $questionId $questionText $smartSession $unifiedModelId
                Write-Output ("MODE_DONE id={0} mode=smart http={1} elapsed_s={2}" -f $questionId, $pair.smart_reasoning.http_status, [math]::Round($pair.smart_reasoning.elapsed_ms / 1000.0, 3))
            }
            $pair.summary = Get-RegressionSummary $pair
            $pair.checks = @(Test-RegressionExpectations $question $pair $pair.summary $config)
            $results.Add($pair)

            $snapshotChecks = @($results | ForEach-Object { @($_.checks) })
            $snapshotFailedCount = @($snapshotChecks | Where-Object { -not $_.passed }).Count
            $snapshot = [ordered]@{
                metadata = [ordered]@{
                    mode = $Mode
                    run_state = "running"
                    started_at = $startedAt
                    base_url = [string]$config.base_url
                    config_path = $resolvedConfigPath
                    questions_path = $resolvedQuestionsPath
                    unified_model_id = $unifiedModelId
                    smart_agent_id = [string]$config.smart_reasoning.agent_id
                    smart_model_id = if ($null -ne $smartBinding) { [string]$smartBinding.TargetModelId } else { "" }
                }
                completed_at = (Get-Date).ToString("o")
                assertion_summary = [ordered]@{
                    passed = $snapshotChecks.Count - $snapshotFailedCount
                    failed = $snapshotFailedCount
                }
                results = $results
            }
            Write-RegressionMarkdownReport $reportPath $Mode $snapshot ([bool]$config.report.include_full_answers)
            Write-Output "QUESTION_DONE id=$questionId report=$reportPath"
        }
    } finally {
        if ($null -ne $client) { $client.Dispose() }
        if ($null -ne $handler) { $handler.Dispose() }
        Restore-SmartModelBinding $config $smartBinding
        Remove-RegressionAuth $config $auth
    }

    $allChecks = @($results | ForEach-Object { @($_.checks) })
    $failedCount = @($allChecks | Where-Object { -not $_.passed }).Count
    $runData = [ordered]@{
        metadata = [ordered]@{
            mode = $Mode
            run_state = "completed"
            started_at = $startedAt
            base_url = [string]$config.base_url
            config_path = $resolvedConfigPath
            questions_path = $resolvedQuestionsPath
            unified_model_id = $unifiedModelId
            smart_agent_id = [string]$config.smart_reasoning.agent_id
            smart_model_id = if ($null -ne $smartBinding) { [string]$smartBinding.TargetModelId } else { "" }
        }
        completed_at = (Get-Date).ToString("o")
        assertion_summary = [ordered]@{ passed = $allChecks.Count - $failedCount; failed = $failedCount }
        results = $results
    }
    Write-RegressionMarkdownReport $reportPath $Mode $runData ([bool]$config.report.include_full_answers)
    Write-Output "ALL_DONE mode=$Mode report=$reportPath failed_assertions=$failedCount"

    if ([bool]$config.assertions.fail_on_failure -and $failedCount -gt 0) {
        throw "Regression assertions failed: $failedCount"
    }
}
