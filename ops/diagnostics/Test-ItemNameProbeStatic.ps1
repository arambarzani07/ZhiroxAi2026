# Pre-release, OFF-PC Windows PowerShell static security and syntax gate.
# This script runs in GitHub Actions, not on the protected market Server.
$ErrorActionPreference='Stop'
$target=Join-Path $PSScriptRoot 'ItemName-ReadOnly-Connection-Gate-v3.ps1'
if(-not (Test-Path -LiteralPath $target -PathType Leaf)){throw 'Probe not found'}
$tokens=$null
$errors=$null
$ast=[System.Management.Automation.Language.Parser]::ParseFile($target,[ref]$tokens,[ref]$errors)
if(@($errors).Count -ne 0){throw ('PowerShell syntax invalid: '+($errors | Select-Object -First 1).Message)}
$src=[System.IO.File]::ReadAllText($target)
if($src -notmatch "SuperMarket" -or $src -notmatch "dbo\.ItemName"){throw 'Expected fixed table/database missing'}
$commandLines=@($src -split "`n" | Where-Object {$_ -match '(&\s+\$sqlcmd\s+-S\s+\$server)'})
if($commandLines.Count -ne 1){throw 'Expected exactly one SQLCMD execution call site'}
$line=$commandLines[0]
foreach($flag in @('-E','-N','-F','-b','-Q','-l','-t')) {
 if($line -notmatch ('(^|\s)'+[regex]::Escape($flag)+'(\s|$)')) {throw ("Missing required SQLCMD switch: "+$flag)}
}
foreach($flag in @('-C','-U','-P','-o')) {
 if($line -match ('(^|\s)'+[regex]::Escape($flag)+'(\s|$)')) {throw ("Forbidden SQLCMD switch: "+$flag)}
}
$queryLines=@($src -split "`n" | Where-Object {$_ -match '^\s*@\{Name='})
if($queryLines.Count -ne 3){throw 'Require exactly three static query statements'}
foreach($q in $queryLines) {
 if($q -notmatch 'SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT') {throw 'Non-allowlisted query in probe'}
 if($q -match '(?i)\b(INSERT|UPDATE|DELETE|MERGE|DROP|ALTER|TRUNCATE|EXEC|EXECUTE|BACKUP|RESTORE)\b') {
  throw 'Mutating SQL was found in query'
 }
}
# The probe itself must not invoke a shell, create/edit files, service controls,
# registry writes, SQL password or certificate validation bypass.
foreach($pattern in @('Invoke-Expression','Start-Process','Set-Item','New-Item','Remove-Item','Invoke-Command','SqlConnection','TrustServerCertificate\s*=\s*true')) {
 if($src -match $pattern){throw ('Forbidden operation in probe: '+$pattern)}
}
Write-Host 'PASS: PowerShell AST parsed, three fixed SELECT-only queries, strict SQLCMD switches, no high-risk calls'
