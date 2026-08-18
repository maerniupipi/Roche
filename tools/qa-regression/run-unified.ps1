param(
    [string]$ConfigPath = (Join-Path $PSScriptRoot "config.json"),
    [string]$QuestionsPath = "",
    [string[]]$QuestionIds = @(),
    [string[]]$Tags = @(),
    [switch]$ValidateOnly
)

. (Join-Path $PSScriptRoot "qa-regression.core.ps1")

Invoke-QARegression `
    -Mode "unified" `
    -ConfigPath $ConfigPath `
    -QuestionsPath $QuestionsPath `
    -QuestionIds $QuestionIds `
    -Tags $Tags `
    -ValidateOnly:$ValidateOnly
