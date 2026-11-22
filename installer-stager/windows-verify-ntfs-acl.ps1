# Read-only Windows NTFS staging preflight. Never changes ACLs, creates
# folders or launches a POS. A failure must block service-side staging.
param(
    [string] $Root = (Join-Path $env:ProgramData 'ZhiroxAI')
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$allowedOwnerSids = @('S-1-5-18', 'S-1-5-32-544') # SYSTEM / Administrators

function Test-ZhiroxSafeContainer([string] $Path) {
    if (-not [IO.Path]::IsPathFullyQualified($Path)) {
        throw 'NTFS_ACL_ROOT_NOT_ABSOLUTE'
    }
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if (-not $item.PSIsContainer) { throw 'NTFS_ACL_EXPECTED_DIRECTORY' }
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw 'NTFS_ACL_REPARSE_POINT_FORBIDDEN'
    }
    $acl = Get-Acl -LiteralPath $Path -ErrorAction Stop
    if (-not $acl.AreAccessRulesProtected) {
        throw 'NTFS_ACL_INHERITANCE_NOT_DISABLED'
    }
    $owner = $acl.GetOwner([System.Security.Principal.SecurityIdentifier]).Value
    if ($allowedOwnerSids -notcontains $owner) {
        throw 'NTFS_ACL_OWNER_NOT_SYSTEM_OR_ADMIN'
    }

    $modifyRights = (
       [System.Security.AccessControl.FileSystemRights]::Modify -bor
       [System.Security.AccessControl.FileSystemRights]::Write -bor
       [System.Security.AccessControl.FileSystemRights]::WriteData -bor
       [System.Security.AccessControl.FileSystemRights]::AppendData -bor
       [System.Security.AccessControl.FileSystemRights]::CreateFiles -bor
       [System.Security.AccessControl.FileSystemRights]::CreateDirectories -bor
       [System.Security.AccessControl.FileSystemRights]::Delete -bor
       [System.Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor
       [System.Security.AccessControl.FileSystemRights]::ChangePermissions -bor
       [System.Security.AccessControl.FileSystemRights]::TakeOwnership
    )
    $rules = $acl.GetAccessRules(
       $true, $true, [System.Security.Principal.SecurityIdentifier])
    foreach ($rule in $rules) {
        if ($rule.AccessControlType -ne
            [System.Security.AccessControl.AccessControlType]::Allow) { continue }
        if (($rule.FileSystemRights -band $modifyRights) -eq 0) { continue }
        if ($allowedOwnerSids -notcontains $rule.IdentityReference.Value) {
            throw 'NTFS_ACL_UNTRUSTED_WRITER'
        }
    }
}

try {
    $root = [IO.Path]::GetFullPath($Root).TrimEnd('\')
    # Never accept a parent or a different production staging location.
    $expected = [IO.Path]::GetFullPath(
        (Join-Path $env:ProgramData 'ZhiroxAI')).TrimEnd('\')
    if (-not [string]::Equals($root,$expected,
        [System.StringComparison]::OrdinalIgnoreCase)) {
        throw 'NTFS_ACL_OUTSIDE_APPROVED_ROOT'
    }
    Test-ZhiroxSafeContainer $root
    Test-ZhiroxSafeContainer (Join-Path $root 'ApprovedInstallers')
    Test-ZhiroxSafeContainer (Join-Path $root 'InstallRequests')
    Write-Output 'NTFS_STAGING_AND_TICKETS_ACCESS_CONTROL_VERIFIED'
    exit 0
} catch {
    # No sensitive paths, identities or file contents are ever logged.
    Write-Error -Message 'NTFS_STAGING_ACCESS_DENIED_OR_NOT_PROVISIONED' -ErrorAction Continue
    exit 17
}
