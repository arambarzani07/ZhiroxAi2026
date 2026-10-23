#!/usr/bin/env python3
"""Regression tests for PC Relay read-only error patch (no PC/SQL access)."""
import unittest
from patch_pc_relay_readonly_errors import patch

ORIGINAL = '''
func fileHashAction(args map[string]interface{}) (interface{}, error) {
    p, err := allowedPath(strArg(args, "path"))
    if err != nil {
        return nil, err
    }
    f, err := os.Open(p)
    if err != nil { return nil, err }
    defer f.Close()
    return nil, nil
}

// wrapper avoids importing crypto/sha256 into main.go
func sha256New() hashWriter { return newSHA256Writer() }

func registryGetAction(args map[string]interface{}) (interface{}, error) {
    p, err := safeRegistryPath(strArg(args, "path"))
    if err != nil { return nil, err }
    return nil, nil
}

func registrySetAction(args map[string]interface{}) (interface{}, error) {
    return nil, errors.New("writes must remain isolated")
}

func registryDeleteAction(args map[string]interface{}) (interface{}, error) {
    return nil, errors.New("writes must remain isolated")
}

func envGetAction(args map[string]interface{}) (interface{}, error) {
    return nil, nil
}
'''

class PatchTests(unittest.TestCase):
    def test_readonly_handlers_and_root_gate(self):
        modified = patch(ORIGINAL)
        self.assertIn('filepath.EvalSymlinks(requested)', modified)
        self.assertIn('target, err := allowedPath(resolved)', modified)
        self.assertIn('FILE_NOT_FOUND:', modified)
        self.assertIn('FILE_ACCESS_DENIED:', modified)
        self.assertIn("code='KEY_NOT_FOUND'", modified)
        self.assertIn("code='VALUE_NOT_FOUND'", modified)
        self.assertNotIn('New-Item ', modified.split('func registrySetAction(', 1)[0])

    def test_preserves_registry_writes_and_idempotency(self):
        modified = patch(ORIGINAL)
        self.assertEqual(
            ORIGINAL.split('func registrySetAction(', 1)[1],
            modified.split('func registrySetAction(', 1)[1],
        )
        self.assertEqual(modified, patch(modified))

    def test_file_hash_after_registry_mutations(self):
        start = ORIGINAL.index("func fileHashAction(")
        registry_start = ORIGINAL.index("func registryGetAction(")
        block = ORIGINAL[start:registry_start]
        reordered = ORIGINAL[:start] + ORIGINAL[registry_start:] + block
        self.assertIn("FILE_NOT_FOUND:", patch(reordered))
        self.assertEqual(patch(reordered), patch(patch(reordered)))

    def test_fail_closed_missing_handlers(self):
        with self.assertRaises(RuntimeError):
            patch(ORIGINAL.replace("func fileHashAction(", "func removedFileHash("))

    def test_fail_closed_duplicate_anchors(self):
        with self.assertRaises(RuntimeError):
            patch(ORIGINAL.replace(
                "// wrapper avoids importing crypto/sha256 into main.go",
                "// wrapper avoids importing crypto/sha256 into main.go\n"
                "// wrapper avoids importing crypto/sha256 into main.go",
            ))

if __name__ == "__main__":
    unittest.main()
