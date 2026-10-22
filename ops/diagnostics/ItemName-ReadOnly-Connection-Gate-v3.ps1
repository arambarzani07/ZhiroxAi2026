# Zhirox ItemName SQL server read-only gate; run only on authorized Windows Server.
# Does not alter SQL schema/data, services, trust stores or app files.
$ErrorActionPreference = 'Continue' # Windows PowerShell 5.1: stderr must be captured, not thrown.
$sqlcmd='C:\Program Files\Microsoft SQL Server\Client SDK\ODBC\180\Tools\Binn\SQLCMD.EXE'
if (-not (Test-Path -LiteralPath $sqlcmd -PathType Leaf)) {
  Write-Output 'BLOCKED: sqlcmd missing'; exit 2
}
$server='tcp:127.0.0.1,1433'
$db='SuperMarket'
$certificateHost='server'
$queries=@(
  @{Name='SCHEMA';SQL="SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT CASE WHEN OBJECT_ID(N'dbo.ItemName',N'U') IS NOT NULL THEN '1' ELSE '0' END;"},
  @{Name='COUNT';SQL='SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT COUNT_BIG(*) FROM dbo.ItemName;'},
  @{Name='FIRST_ID';SQL='SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT TOP(1) CONVERT(VARCHAR(40),ItemNameID) FROM dbo.ItemName ORDER BY ItemNameID;'}
)
$results=@{}
foreach($q in $queries) {
  $out=@(& $sqlcmd -S $server -E -N -F $certificateHost -d $db -b -W -h -1 -l 10 -t 35 -r 1 -Q $q.SQL 2>&1)
  $code=$LASTEXITCODE
  if($code -ne 0) {
    Write-Output ("BLOCKED: {0} (exit={1})" -f $q.Name,$code)
    $message=(@($out | ForEach-Object { [string]$_ }) -join ' ').ToLowerInvariant()
    $category='SQLCMD_ERROR_UNCLASSIFIED'
    if($message -match 'target principal name|hostname in certificate|certificate name mismatch'){$category='SQL_TLS_CERT_NAME_MISMATCH'}
    elseif($message -match 'certificate chain|certificate verify|self-signed'){$category='SQL_TLS_CERT_TRUST'}
    elseif($message -match 'ssl provider|tls handshake|handshake failure|protocol version'){$category='SQL_TLS_HANDSHAKE'}
    elseif($message -match 'login failed|18456'){$category='SQL_INTEGRATED_LOGIN_FAILED'}
    elseif($message -match 'cannot open database|4060'){$category='SQL_DATABASE_ACCESS_DENIED'}
    elseif($message -match 'network-related|server was not found|08001|connection timeout'){$category='SQL_CONNECTION_FAILED'}
    elseif($message -match 'invalid column name|invalid object name|permission denied'){$category='SQL_SCHEMA_OR_PERMISSION_ERROR'}
    Write-Output ("SQLCMD_ERROR_CATEGORY="+$category)
    Write-Output 'LEGACY_DB_CHANGED=NO'
    exit 3
  }
  $nums=@($out | ForEach-Object { [string]$_ } |
    ForEach-Object { $_.Trim() } | Where-Object { $_ -match '^\d+$' })
  if($nums.Count -ne 1) {Write-Output ("BLOCKED: AMBIGUOUS_{0}" -f $q.Name);exit 4}
  $results[$q.Name]=$nums[0]
}
if($results.SCHEMA -ne '1') { Write-Output 'BLOCKED: ItemName inaccessible';exit 5 }
Write-Output ("PASS: LIVE_SELECT=true; DB=SuperMarket; ItemName_ROWS={0}; FIRST_ID={1}" -f $results.COUNT,$results.FIRST_ID)
Write-Output 'ENCRYPTION=-N; CERT_HOST=server; TRUST_BYPASS=NO; LEGACY_DB_CHANGED=NO'
