//go:build windows

package main

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	pcOSCoreServiceName = "ZhiroxAICore"

	pcOSSCManagerConnect      = 0x0001
	pcOSServiceQueryConfig    = 0x0001
	pcOSServiceChangeConfig   = 0x0002
	pcOSServiceConfigFailure  = 2
	pcOSServiceConfigFailFlag = 4

	pcOSSCActionNone    = 0
	pcOSSCActionRestart = 1
)

type pcOSSCAction struct {
	Type  uint32
	Delay uint32
}

type pcOSServiceFailureActions struct {
	ResetPeriod  uint32
	RebootMsg    *uint16
	Command      *uint16
	ActionsCount uint32
	Actions      *pcOSSCAction
}

type pcOSServiceFailureActionsFlag struct {
	FailureActionsOnNonCrashFailures int32
}

var (
	pcOSAdvapi32                    = syscall.NewLazyDLL("advapi32.dll")
	pcOSOpenSCManagerW              = pcOSAdvapi32.NewProc("OpenSCManagerW")
	pcOSOpenServiceW                = pcOSAdvapi32.NewProc("OpenServiceW")
	pcOSCloseServiceHandle          = pcOSAdvapi32.NewProc("CloseServiceHandle")
	pcOSChangeServiceConfig2W       = pcOSAdvapi32.NewProc("ChangeServiceConfig2W")
	pcOSQueryServiceConfig2W        = pcOSAdvapi32.NewProc("QueryServiceConfig2W")
)

func pcOSWinAPIError(op string, callErr error) error {
	if errno, ok := callErr.(syscall.Errno); ok && errno != 0 {
		return fmt.Errorf("%s: %w", op, errno)
	}
	return errors.New(op + " failed")
}

func pcOSOpenCoreService(access uintptr) (uintptr, func(), error) {
	scm, _, err := pcOSOpenSCManagerW.Call(0, 0, pcOSSCManagerConnect)
	if scm == 0 {
		return 0, nil, pcOSWinAPIError("OpenSCManagerW", err)
	}
	cleanupSCM := func() { pcOSCloseServiceHandle.Call(scm) }

	name, convErr := syscall.UTF16PtrFromString(pcOSCoreServiceName)
	if convErr != nil {
		cleanupSCM()
		return 0, nil, convErr
	}
	svc, _, err := pcOSOpenServiceW.Call(scm, uintptr(unsafe.Pointer(name)), access)
	if svc == 0 {
		cleanupSCM()
		return 0, nil, pcOSWinAPIError("OpenServiceW", err)
	}
	cleanup := func() {
		pcOSCloseServiceHandle.Call(svc)
		cleanupSCM()
	}
	return svc, cleanup, nil
}

func pcOSQueryConfig2(svc uintptr, level uintptr) ([]byte, error) {
	var needed uint32
	r1, _, callErr := pcOSQueryServiceConfig2W.Call(
		svc,
		level,
		0,
		0,
		uintptr(unsafe.Pointer(&needed)),
	)
	if r1 == 0 && needed == 0 {
		return nil, pcOSWinAPIError("QueryServiceConfig2W(size)", callErr)
	}
	if needed == 0 {
		return nil, errors.New("QueryServiceConfig2W returned an empty configuration")
	}

	buf := make([]byte, needed)
	r1, _, callErr = pcOSQueryServiceConfig2W.Call(
		svc,
		level,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r1 == 0 {
		return nil, pcOSWinAPIError("QueryServiceConfig2W", callErr)
	}
	return buf, nil
}

func pcOSRecoveryActionName(t uint32) string {
	switch t {
	case pcOSSCActionRestart:
		return "restart"
	case pcOSSCActionNone:
		return "none"
	default:
		return fmt.Sprintf("type_%d", t)
	}
}

func pcOSCoreRecoveryStatus() (interface{}, error) {
	svc, cleanup, err := pcOSOpenCoreService(pcOSServiceQueryConfig)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	failureBuf, err := pcOSQueryConfig2(svc, pcOSServiceConfigFailure)
	if err != nil {
		return nil, err
	}
	cfg := (*pcOSServiceFailureActions)(unsafe.Pointer(&failureBuf[0]))
	actions := make([]map[string]interface{}, 0, int(cfg.ActionsCount))
	if cfg.Actions != nil && cfg.ActionsCount > 0 && cfg.ActionsCount <= 32 {
		for _, action := range unsafe.Slice(cfg.Actions, int(cfg.ActionsCount)) {
			actions = append(actions, map[string]interface{}{
				"action":   pcOSRecoveryActionName(action.Type),
				"delay_ms": action.Delay,
			})
		}
	}

	nonCrash := false
	if flagBuf, flagErr := pcOSQueryConfig2(svc, pcOSServiceConfigFailFlag); flagErr == nil && len(flagBuf) >= 4 {
		flag := (*pcOSServiceFailureActionsFlag)(unsafe.Pointer(&flagBuf[0]))
		nonCrash = flag.FailureActionsOnNonCrashFailures != 0
	}

	return map[string]interface{}{
		"module":                       "core_self_healing",
		"service":                      pcOSCoreServiceName,
		"reset_period_seconds":         cfg.ResetPeriod,
		"actions":                      actions,
		"failure_actions_non_crash":    nonCrash,
		"reboot_action_configured":     false,
		"external_command_configured":  false,
		"windows_reboot_allowed":       false,
		"zero_ui":                      true,
		"generic_core_restart_locked":  true,
	}, nil
}

func pcOSRecoveryBool(args map[string]interface{}, key string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func pcOSSetCoreRecovery(args map[string]interface{}) (interface{}, error) {
	if !pcOSRecoveryBool(args, "approved") || !pcOSRecoveryBool(args, "owner_command") {
		return nil, errors.New("core self-healing requires explicit owner approval")
	}
	if strings.TrimSpace(fmt.Sprint(args["confirm"])) != "ENABLE_ZHIROX_CORE_SELF_HEALING" {
		return nil, errors.New("core self-healing confirmation mismatch")
	}

	svc, cleanup, err := pcOSOpenCoreService(pcOSServiceQueryConfig | pcOSServiceChangeConfig)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Three bounded restart attempts, then fail closed. The last NONE action is
	// repeated by SCM for subsequent failures until the reset period expires.
	actions := []pcOSSCAction{
		{Type: pcOSSCActionRestart, Delay: 10_000},
		{Type: pcOSSCActionRestart, Delay: 30_000},
		{Type: pcOSSCActionRestart, Delay: 60_000},
		{Type: pcOSSCActionNone, Delay: 0},
	}
	failure := pcOSServiceFailureActions{
		ResetPeriod:  24 * 60 * 60,
		RebootMsg:    nil,
		Command:      nil,
		ActionsCount: uint32(len(actions)),
		Actions:      &actions[0],
	}
	r1, _, callErr := pcOSChangeServiceConfig2W.Call(
		svc,
		pcOSServiceConfigFailure,
		uintptr(unsafe.Pointer(&failure)),
	)
	if r1 == 0 {
		return nil, pcOSWinAPIError("ChangeServiceConfig2W(failure_actions)", callErr)
	}

	flag := pcOSServiceFailureActionsFlag{FailureActionsOnNonCrashFailures: 1}
	r1, _, callErr = pcOSChangeServiceConfig2W.Call(
		svc,
		pcOSServiceConfigFailFlag,
		uintptr(unsafe.Pointer(&flag)),
	)
	if r1 == 0 {
		return nil, pcOSWinAPIError("ChangeServiceConfig2W(failure_flag)", callErr)
	}

	return pcOSCoreRecoveryStatus()
}
