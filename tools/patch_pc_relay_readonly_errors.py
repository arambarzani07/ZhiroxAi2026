#!/usr/bin/env python3
"""Patch only read-only PC Relay actions in restored Supervisor source.

The original source is restored from source-v5.0.1.partNNN during GitHub Actions.
Do not change allowedPath(), cfg.AllowedRoots, or registry write commands.
Fails closed if the expected source anchors are missing or ambiguous.
"""
from pathlib import Path

SOURCE = Path("buildsrc/admin_actions_v48.go")

FILE_HASH = r'''func fileHashAction(args map[string]interface{}) (interface{}, error) {
    requested, err := allowedPath(strArg(args, "path"))
    if err != nil {
        return nil, err
    }
    // The lexical allowed_roots gate above is mandatory. Canonicalize again
    // and re-check the resolved target so a link cannot escape allowed_roots.
    resolved, err := filepath.EvalSymlinks(requested)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, fmt.Errorf("FILE_NOT_FOUND: %w", err)
        }
        if os.IsPermission(err) {
            return nil, fmt.Errorf("FILE_ACCESS_DENIED: %w", err)
        }
        return nil, fmt.Errorf("FILE_RESOLUTION_FAILED: %w", err)
    }
    target, err := allowedPath(resolved)
    if err != nil {
        return nil, fmt.Errorf("PATH_OUTSIDE_ALLOWED_ROOTS: %w", err)
    }
    f, err := os.Open(target)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, fmt.Errorf("FILE_NOT_FOUND: %w", err)
        }
        if os.IsPermission(err) {
            return nil, fmt.Errorf("FILE_ACCESS_DENIED: %w", err)
        }
        return nil, fmt.Errorf("FILE_OPEN_FAILED: %w", err)
    }
    defer f.Close()
    info, err := f.Stat()
    if err != nil {
        return nil, fmt.Errorf("FILE_STAT_FAILED: %w", err)
    }
    if !info.Mode().IsRegular() {
        return nil, errors.New("FILE_NOT_REGULAR")
    }
    h := sha256New()
    n, err := io.Copy(h, f)
    if err != nil {
        return nil, fmt.Errorf("FILE_READ_FAILED: %w", err)
    }
    return map[string]interface{}{"path": requested, "bytes": n, "sha256": hex.EncodeToString(h.Sum(nil))}, nil
}
'''

REGISTRY_GET = r'''func registryGetAction(args map[string]interface{}) (interface{}, error) {
    p, err := safeRegistryPath(strArg(args, "path"))
    if err != nil {
        return nil, err
    }
    name := strArg(args, "name")
    // An optional Windows registry key may legitimately be absent. Report
    // absence as structured read-only data instead of throwing a PS error.
    script := fmt.Sprintf(`$p=%s;if(!(Test-Path -LiteralPath $p)){[pscustomobject]@{path=$p;exists=$false;code='KEY_NOT_FOUND'}|ConvertTo-Json -Compress;return};`, psQuote(p))
    if name == "" {
        script += `$i=Get-Item -LiteralPath $p -ErrorAction Stop;[pscustomobject]@{path=$p;subkeys=@($i.GetSubKeyNames());values=@($i.GetValueNames())}|ConvertTo-Json -Compress`
    } else {
        script += fmt.Sprintf(`$n=%s;$props=Get-ItemProperty -LiteralPath $p -ErrorAction Stop;$pr=$props.PSObject.Properties[$n];if($null -eq $pr){[pscustomobject]@{path=$p;name=$n;exists=$false;code='VALUE_NOT_FOUND'}|ConvertTo-Json -Compress;return};[pscustomobject]@{path=$p;name=$n;value=$pr.Value}|ConvertTo-Json -Compress`, psQuote(name))
    }
    return jsonPowerShell(script, 30*time.Second)
}
'''

def replace_block(text: str, begin: str, end: str, replacement: str) -> str:
    if text.count(begin) != 1 or text.count(end) != 1:
        raise RuntimeError("Fail closed: unrecognized or ambiguous Supervisor source anchors")
    start = text.index(begin)
    stop = text.index(end, start)
    if stop <= start:
        raise RuntimeError("Fail closed: unexpected function ordering")
    return text[:start] + replacement.rstrip() + "\n\n" + text[stop:]

def patch(text: str) -> str:
    if 'func fileHashAction(args map[string]interface{})' not in text:
        raise RuntimeError("fileHashAction missing")
    if 'func registryGetAction(args map[string]interface{})' not in text:
        raise RuntimeError("registryGetAction missing")
    # No changes to allowedPath, safeRegistryPath, or registrySet/Delete.
    original_registry_tail = text.split("func registrySetAction(", 1)[1]
    if 'FILE_RESOLUTION_FAILED:' not in text:
        text = replace_block(text, "func fileHashAction(", "// wrapper avoids importing crypto/sha256", FILE_HASH)
    if "code='KEY_NOT_FOUND'" not in text:
        text = replace_block(text, "func registryGetAction(", "func registrySetAction(", REGISTRY_GET)
    if text.split("func registrySetAction(", 1)[1] != original_registry_tail:
        raise RuntimeError("Mutation code unexpectedly changed")
    for marker in ("allowedPath(strArg(args, \"path\"))", "filepath.EvalSymlinks(requested)",
                   "target, err := allowedPath(resolved)", "KEY_NOT_FOUND", "VALUE_NOT_FOUND"):
        if marker not in text:
            raise RuntimeError("Missing safety invariant: " + marker)
    return text

def main() -> None:
    current = SOURCE.read_text(encoding="utf-8")
    updated = patch(current)
    if updated == current:
        print("Read-only error classification already patched")
        return
    SOURCE.write_text(updated, encoding="utf-8")
    print("Applied read-only file_hash/registry_get classification; allowed_roots unchanged")

if __name__ == "__main__":
    main()
