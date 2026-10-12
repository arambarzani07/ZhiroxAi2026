package main

import (
	"errors"
	"strings"
)

const (
	pcOSKurdishVoiceLocale         = "ckb-IQ"
	pcOSKurdishVoiceDefaultEnabled = false
	pcOSKurdishVoiceProvider       = "local_wav_event_pack"
)

type pcOSKurdishVoiceEntry struct {
	Event  string
	Phrase string
	Asset  string
}

var pcOSKurdishVoiceEntries = map[string]pcOSKurdishVoiceEntry{
	"system_ready": {
		Event:  "system_ready",
		Phrase: "سیستەمەکە ئامادەیە.",
		Asset:  "system-ready.wav",
	},
	"update_success": {
		Event:  "update_success",
		Phrase: "نوێکردنەوە بە سەرکەوتوویی تەواو بوو.",
		Asset:  "update-success.wav",
	},
	"internet_problem": {
		Event:  "internet_problem",
		Phrase: "کێشەیەک لە پەیوەندی ئینتەرنێت هەیە.",
		Asset:  "internet-problem.wav",
	},
	"disk_low": {
		Event:  "disk_low",
		Phrase: "شوێنی بەردەست لە دیسک کەمە.",
		Asset:  "disk-low.wav",
	},
	"service_recovered": {
		Event:  "service_recovered",
		Phrase: "خزمەتگوزاری زیڕۆکس دووبارە چالاک کرایەوە.",
		Asset:  "service-recovered.wav",
	},
	"backup_success": {
		Event:  "backup_success",
		Phrase: "پاشەکەوتکردن بە سەرکەوتوویی تەواو بوو.",
		Asset:  "backup-success.wav",
	},
	"security_warning": {
		Event:  "security_warning",
		Phrase: "ئاگادارییەکی ئاسایشی هەیە.",
		Asset:  "security-warning.wav",
	},
}

func pcOSKurdishVoicePolicy() map[string]interface{} {
	return map[string]interface{}{
		"module":                  "kurdish_voice",
		"locale":                  pcOSKurdishVoiceLocale,
		"provider":                pcOSKurdishVoiceProvider,
		"default_enabled":         pcOSKurdishVoiceDefaultEnabled,
		"zero_ui":                 true,
		"microphone_enabled":      false,
		"continuous_listening":    false,
		"network_required":        false,
		"shell_allowed":           false,
		"free_form_tts_enabled":   false,
		"event_allowlist_required": true,
		"asset_root_mutable":      false,
	}
}

func pcOSKurdishVoiceLookup(event string) (pcOSKurdishVoiceEntry, error) {
	event = strings.TrimSpace(strings.ToLower(event))
	entry, ok := pcOSKurdishVoiceEntries[event]
	if !ok {
		return pcOSKurdishVoiceEntry{}, errors.New("unsupported Kurdish voice event")
	}
	if entry.Asset == "" || strings.ContainsAny(entry.Asset, `/\\:`) || !strings.HasSuffix(strings.ToLower(entry.Asset), ".wav") {
		return pcOSKurdishVoiceEntry{}, errors.New("invalid Kurdish voice asset mapping")
	}
	return entry, nil
}
