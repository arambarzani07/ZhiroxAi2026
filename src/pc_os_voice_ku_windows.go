//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const pcOSKurdishVoiceAssetRoot = `C:\ProgramData\ZhiroxAI\voice\ku`

const (
	pcOSSNDAsync     = 0x0001
	pcOSSNDNoDefault = 0x0002
	pcOSSNDFilename  = 0x00020000
)

var (
	pcOSWinMM      = syscall.NewLazyDLL("winmm.dll")
	pcOSPlaySoundW = pcOSWinMM.NewProc("PlaySoundW")
)

func pcOSPlayKurdishVoiceEvent(event string) error {
	entry, err := pcOSKurdishVoiceLookup(event)
	if err != nil {
		return err
	}

	assetPath := filepath.Join(pcOSKurdishVoiceAssetRoot, entry.Asset)
	cleanRoot := filepath.Clean(pcOSKurdishVoiceAssetRoot)
	cleanAsset := filepath.Clean(assetPath)
	if filepath.Dir(cleanAsset) != cleanRoot {
		return errors.New("Kurdish voice asset escaped protected root")
	}

	info, err := os.Stat(cleanAsset)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("Kurdish voice asset is not a file")
	}

	pathPtr, err := syscall.UTF16PtrFromString(cleanAsset)
	if err != nil {
		return err
	}
	flags := uintptr(pcOSSNDAsync | pcOSSNDNoDefault | pcOSSNDFilename)
	ret, _, callErr := pcOSPlaySoundW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, flags)
	if ret == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return errors.New("PlaySoundW rejected Kurdish voice asset")
	}
	return nil
}
