package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeBackupFixture(t *testing.T, path string, data []byte, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareReadOnlyExportRequiresExplicitApproval(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "SuperMarket_backup_2026_10_16_120004_7456635.bak")
	writeBackupFixture(t, source, []byte("backup"), time.Now())
	_, err := pcOSPrepareReadOnlyExportForRoots(source, root, filepath.Join(t.TempDir(), "exports"), false, time.Now())
	if err == nil || !strings.Contains(err.Error(), "explicit approval") {
		t.Fatalf("expected explicit approval rejection, got %v", err)
	}
}

func TestPrepareReadOnlyExportRejectsOutsideRoot(t *testing.T) {
	allowed := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.bak")
	writeBackupFixture(t, outside, []byte("backup"), time.Now())
	_, err := pcOSPrepareReadOnlyExportForRoots(outside, allowed, filepath.Join(t.TempDir(), "exports"), true, time.Now())
	if err == nil || !strings.Contains(err.Error(), "outside the approved backup root") {
		t.Fatalf("expected outside-root rejection, got %v", err)
	}
}

func TestPrepareReadOnlyExportRejectsNonBackupExtension(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "backup.zip")
	writeBackupFixture(t, source, []byte("backup"), time.Now())
	_, err := pcOSPrepareReadOnlyExportForRoots(source, root, filepath.Join(t.TempDir(), "exports"), true, time.Now())
	if err == nil || !strings.Contains(err.Error(), "only .bak") {
		t.Fatalf("expected extension rejection, got %v", err)
	}
}

func TestPrepareReadOnlyExportStagesVerifiedCopyWithoutTransfer(t *testing.T) {
	root := t.TempDir()
	exportRoot := filepath.Join(t.TempDir(), "exports")
	source := filepath.Join(root, "SuperMarket_backup_2026_10_16_120004_7456635.bak")
	content := []byte("immutable database backup fixture")
	modified := time.Date(2026, 10, 16, 9, 0, 6, 0, time.UTC)
	writeBackupFixture(t, source, content, modified)
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)
	manifest, err := pcOSPrepareReadOnlyExportForRoots(source, root, exportRoot, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.ReadOnly || !manifest.ApprovalRequired || manifest.TransferEnabled || manifest.NetworkUpload {
		t.Fatalf("unsafe export flags: %#v", manifest)
	}
	if manifest.Size != int64(len(content)) {
		t.Fatalf("unexpected size: %d", manifest.Size)
	}
	if len(manifest.SHA256) != 64 {
		t.Fatalf("expected SHA-256, got %q", manifest.SHA256)
	}
	if manifest.SourcePath == manifest.StagedPath {
		t.Fatal("source and staged paths must differ")
	}
	staged, err := os.ReadFile(manifest.StagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(staged) != string(content) {
		t.Fatal("staged copy differs from source")
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("source backup was modified")
	}
	raw, err := os.ReadFile(manifest.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted pcOSReadOnlyExportManifest
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.ExportID != manifest.ExportID || persisted.SHA256 != manifest.SHA256 {
		t.Fatalf("persisted manifest mismatch: %#v", persisted)
	}
	if persisted.TransferEnabled || persisted.NetworkUpload {
		t.Fatal("manifest must never enable transfer or network upload")
	}
}

func TestFindLatestSuperMarketBackup(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	writeBackupFixture(t, filepath.Join(root, "SuperMarket_backup_2026_10_16_080000_1111111.bak"), []byte("old"), base)
	newest := filepath.Join(root, "SuperMarket_backup_2026_10_16_120000_2222222.bak")
	writeBackupFixture(t, newest, []byte("new"), base.Add(4*time.Hour))
	writeBackupFixture(t, filepath.Join(root, "Other_backup_2026_10_16_130000.bak"), []byte("ignore"), base.Add(5*time.Hour))
	writeBackupFixture(t, filepath.Join(root, "SuperMarket_backup_2026_10_16_140000.zip"), []byte("ignore"), base.Add(6*time.Hour))
	got, err := pcOSFindLatestSuperMarketBackup(root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedNewest, _ := filepath.EvalSymlinks(newest)
	if got != filepath.Clean(resolvedNewest) {
		t.Fatalf("got %q want %q", got, resolvedNewest)
	}
}

func TestPrepareReadOnlyExportRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require additional Windows privilege")
	}
	allowed := t.TempDir()
	outsideRoot := t.TempDir()
	outside := filepath.Join(outsideRoot, "outside.bak")
	writeBackupFixture(t, outside, []byte("backup"), time.Now())
	link := filepath.Join(allowed, "linked.bak")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err := pcOSPrepareReadOnlyExportForRoots(link, allowed, filepath.Join(t.TempDir(), "exports"), true, time.Now())
	if err == nil || !strings.Contains(err.Error(), "outside the approved backup root") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
}

func TestPrepareReadOnlyExportRejectsOversizeBeforeCopy(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "oversize.bak")
	f, err := os.OpenFile(source, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(pcOSReadOnlyExportMaxBytes + 1); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = pcOSPrepareReadOnlyExportForRoots(source, root, filepath.Join(t.TempDir(), "exports"), true, time.Now())
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversize rejection, got %v", err)
	}
}
