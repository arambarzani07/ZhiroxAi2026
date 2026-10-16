package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	pcOSReadOnlyExportMaxBytes int64 = 512 * 1024 * 1024
	pcOSReadOnlyExportVersion        = "read-only-backup-export/v1"
)

type pcOSReadOnlyExportManifest struct {
	Version          string `json:"version"`
	ExportID         string `json:"export_id"`
	SourcePath       string `json:"source_path"`
	StagedPath       string `json:"staged_path"`
	ManifestPath     string `json:"manifest_path"`
	Size             int64  `json:"size"`
	SHA256           string `json:"sha256"`
	SourceModified   string `json:"source_modified_utc"`
	CreatedUTC       string `json:"created_utc"`
	ReadOnly         bool   `json:"read_only"`
	ApprovalRequired bool   `json:"approval_required"`
	TransferEnabled  bool   `json:"transfer_enabled"`
	NetworkUpload    bool   `json:"network_upload"`
}

func pcOSReadOnlyExportActiveRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Clean(`C:\ProgramData\ZhiroxAI`)
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return filepath.Clean(`C:\ProgramData\ZhiroxAI`)
	}
	return filepath.Clean(filepath.Dir(abs))
}

func pcOSReadOnlyExportPathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func pcOSReadOnlyExportResolveExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func pcOSReadOnlyExportRandomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func pcOSReadOnlyExportHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func pcOSReadOnlyExportValidateSource(sourcePath, allowedRoot string) (string, os.FileInfo, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", nil, errors.New("source path is required")
	}
	root, err := pcOSReadOnlyExportResolveExisting(allowedRoot)
	if err != nil {
		return "", nil, fmt.Errorf("resolve allowed backup root: %w", err)
	}
	source, err := pcOSReadOnlyExportResolveExisting(sourcePath)
	if err != nil {
		return "", nil, fmt.Errorf("resolve source backup: %w", err)
	}
	if !pcOSReadOnlyExportPathWithin(source, root) {
		return "", nil, errors.New("source backup is outside the approved backup root")
	}
	info, err := os.Stat(source)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, errors.New("source backup must be a regular file")
	}
	if !strings.EqualFold(filepath.Ext(source), ".bak") {
		return "", nil, errors.New("only .bak backup files may be staged")
	}
	if info.Size() <= 0 {
		return "", nil, errors.New("source backup is empty")
	}
	if info.Size() > pcOSReadOnlyExportMaxBytes {
		return "", nil, fmt.Errorf("source backup exceeds %d byte export limit", pcOSReadOnlyExportMaxBytes)
	}
	return source, info, nil
}

func pcOSPrepareReadOnlyExportForRoots(sourcePath, allowedRoot, exportRoot string, explicitlyApproved bool, now time.Time) (pcOSReadOnlyExportManifest, error) {
	var zero pcOSReadOnlyExportManifest
	if !explicitlyApproved {
		return zero, errors.New("explicit approval is required before staging a backup export")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	source, info, err := pcOSReadOnlyExportValidateSource(sourcePath, allowedRoot)
	if err != nil {
		return zero, err
	}

	exportRootAbs, err := filepath.Abs(exportRoot)
	if err != nil {
		return zero, err
	}
	exportRootAbs = filepath.Clean(exportRootAbs)
	if err := os.MkdirAll(exportRootAbs, 0700); err != nil {
		return zero, fmt.Errorf("create export staging root: %w", err)
	}

	exportID, err := pcOSReadOnlyExportRandomID()
	if err != nil {
		return zero, fmt.Errorf("create export id: %w", err)
	}
	stageDir := filepath.Join(exportRootAbs, exportID)
	if err := os.Mkdir(stageDir, 0700); err != nil {
		return zero, fmt.Errorf("create export staging directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stageDir)
		}
	}()

	destination := filepath.Join(stageDir, filepath.Base(source))
	src, err := os.Open(source)
	if err != nil {
		return zero, err
	}
	defer src.Close()

	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return zero, err
	}
	h := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, pcOSReadOnlyExportMaxBytes+1))
	syncErr := dst.Sync()
	closeErr := dst.Close()
	if copyErr != nil {
		return zero, fmt.Errorf("stage backup copy: %w", copyErr)
	}
	if syncErr != nil {
		return zero, fmt.Errorf("sync staged backup: %w", syncErr)
	}
	if closeErr != nil {
		return zero, fmt.Errorf("close staged backup: %w", closeErr)
	}
	if written != info.Size() {
		return zero, fmt.Errorf("staged backup size mismatch: got %d want %d", written, info.Size())
	}
	if written > pcOSReadOnlyExportMaxBytes {
		return zero, errors.New("staged backup exceeded export size limit")
	}

	sourceHash := hex.EncodeToString(h.Sum(nil))
	stagedHash, err := pcOSReadOnlyExportHash(destination)
	if err != nil {
		return zero, fmt.Errorf("verify staged backup hash: %w", err)
	}
	if !strings.EqualFold(sourceHash, stagedHash) {
		return zero, errors.New("staged backup SHA-256 mismatch")
	}
	_ = os.Chmod(destination, 0400)

	manifestPath := filepath.Join(stageDir, "manifest.json")
	manifest := pcOSReadOnlyExportManifest{
		Version:          pcOSReadOnlyExportVersion,
		ExportID:         exportID,
		SourcePath:       source,
		StagedPath:       destination,
		ManifestPath:     manifestPath,
		Size:             written,
		SHA256:           stagedHash,
		SourceModified:   info.ModTime().UTC().Format(time.RFC3339Nano),
		CreatedUTC:       now.Format(time.RFC3339Nano),
		ReadOnly:         true,
		ApprovalRequired: true,
		TransferEnabled:  false,
		NetworkUpload:    false,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return zero, err
	}
	if err := os.WriteFile(manifestPath, raw, 0600); err != nil {
		return zero, fmt.Errorf("write export manifest: %w", err)
	}
	_ = os.Chmod(manifestPath, 0400)

	cleanup = false
	return manifest, nil
}

func pcOSFindLatestSuperMarketBackup(root string) (string, error) {
	resolvedRoot, err := pcOSReadOnlyExportResolveExisting(root)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(resolvedRoot)
	if err != nil {
		return "", err
	}
	var latestPath string
	var latestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "supermarket_backup_") || !strings.HasSuffix(lower, ".bak") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > pcOSReadOnlyExportMaxBytes {
			continue
		}
		if latestPath == "" || info.ModTime().After(latestTime) {
			latestPath = filepath.Join(resolvedRoot, name)
			latestTime = info.ModTime()
		}
	}
	if latestPath == "" {
		return "", errors.New("no eligible SuperMarket backup found")
	}
	return latestPath, nil
}

func pcOSPrepareLatestSuperMarketBackupExport(explicitlyApproved bool) (pcOSReadOnlyExportManifest, error) {
	backupRoot := filepath.Clean(`D:\Backup`)
	latest, err := pcOSFindLatestSuperMarketBackup(backupRoot)
	if err != nil {
		return pcOSReadOnlyExportManifest{}, err
	}
	exportRoot := filepath.Join(pcOSReadOnlyExportActiveRoot(), "exports")
	return pcOSPrepareReadOnlyExportForRoots(latest, backupRoot, exportRoot, explicitlyApproved, time.Now().UTC())
}
