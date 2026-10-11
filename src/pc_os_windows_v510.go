package main

import "time"

func pcOSCapture(label string, fn func() (interface{}, error)) map[string]interface{} {
	out := map[string]interface{}{"label": label}
	value, err := fn()
	if err != nil {
		out["ok"] = false
		out["error"] = err.Error()
		return out
	}
	out["ok"] = true
	out["data"] = value
	return out
}

func pcOSWindowsInventory() (interface{}, error) {
	return map[string]interface{}{
		"module":          "inventory_asset_manager",
		"generated_utc":   time.Now().UTC().Format(time.RFC3339),
		"read_only":       true,
		"zero_ui":         true,
		"hardware":        pcOSCapture("hardware", hardwareInventory),
		"disks":           pcOSCapture("disks", diskInventory),
		"network_adapters": pcOSCapture("network_adapters", networkAdapters),
		"drivers":         pcOSCapture("drivers", driverInventory),
		"startup_items":   pcOSCapture("startup_items", startupItems),
		"scheduled_tasks": pcOSCapture("scheduled_tasks", scheduledTasks),
		"windows_update":  pcOSCapture("windows_update", windowsUpdateStatus),
		"security":        pcOSCapture("security", securityStatus),
	}, nil
}

func pcOSWindowsGuardian() (interface{}, error) {
	metrics, metricsErr := metricsSnapshot()
	guardian, _ := pcOSGuardian()
	result := map[string]interface{}{
		"module":        "guardian",
		"generated_utc": time.Now().UTC().Format(time.RFC3339),
		"read_only":     true,
		"zero_ui":       true,
		"critical_lock": true,
		"guardian":      guardian,
		"cashier": pcOSCapture("Cashier", func() (interface{}, error) {
			return appStatus("Cashier")
		}),
		"admin": pcOSCapture("Admin", func() (interface{}, error) {
			return appStatus("Admin")
		}),
		"services": pcOSCapture("services", serviceList),
		"processes": pcOSCapture("processes", processList),
	}
	if metricsErr != nil {
		result["metrics_error"] = metricsErr.Error()
	} else {
		score, alerts := healthFromMetrics(metrics)
		result["metrics"] = metrics
		result["health_score"] = score
		result["alerts"] = alerts
	}
	return result, nil
}

func pcOSWindowsContinuity() (interface{}, error) {
	base, _ := pcOSBusinessContinuity()
	return map[string]interface{}{
		"module":          "business_continuity",
		"generated_utc":   time.Now().UTC().Format(time.RFC3339),
		"read_only":       true,
		"zero_ui":         true,
		"policy":          base,
		"cashier": pcOSCapture("Cashier", func() (interface{}, error) {
			return appStatus("Cashier")
		}),
		"admin": pcOSCapture("Admin", func() (interface{}, error) {
			return appStatus("Admin")
		}),
		"services":        pcOSCapture("services", serviceList),
		"network":         pcOSCapture("network", networkList),
		"recent_errors":   pcOSCapture("recent_event_errors", recentEventErrors),
		"automatic_mutation": false,
	}, nil
}

func pcOSWindowsDiskIntelligence() (interface{}, error) {
	base, _ := pcOSDiskIntelligence()
	return map[string]interface{}{
		"module":        "disk_intelligence",
		"generated_utc": time.Now().UTC().Format(time.RFC3339),
		"read_only":     true,
		"disk_inventory": pcOSCapture("disk_inventory", diskInventory),
		"bounded_scan":  base,
		"delete_unknown_files": false,
		"warning_free_percent": 15,
		"critical_free_percent": 10,
	}, nil
}

func pcOSWindowsSecurity() (interface{}, error) {
	base, _ := pcOSSecurity()
	return map[string]interface{}{
		"module":          "security_sentinel",
		"generated_utc":   time.Now().UTC().Format(time.RFC3339),
		"read_only":       true,
		"zero_ui":         true,
		"base_integrity":  base,
		"security_status": pcOSCapture("security_status", securityStatus),
		"windows_update":  pcOSCapture("windows_update", windowsUpdateStatus),
		"startup_items":   pcOSCapture("startup_items", startupItems),
		"scheduled_tasks": pcOSCapture("scheduled_tasks", scheduledTasks),
		"drivers":         pcOSCapture("drivers", driverInventory),
		"recent_errors":   pcOSCapture("recent_event_errors", recentEventErrors),
		"automatic_quarantine": false,
		"automatic_firewall_change": false,
	}, nil
}

func pcOSWindowsAI() (interface{}, error) {
	base, _ := pcOSLocalAI()
	result := map[string]interface{}{
		"module":                "local_ai_operator",
		"generated_utc":         time.Now().UTC().Format(time.RFC3339),
		"engine":                "deterministic safety rules + live Windows telemetry",
		"read_only":             true,
		"autonomous_destructive_actions": false,
		"base_findings":         base,
		"recent_errors":         pcOSCapture("recent_event_errors", recentEventErrors),
	}
	if metrics, err := metricsSnapshot(); err == nil {
		score, alerts := healthFromMetrics(metrics)
		result["metrics"] = metrics
		result["health_score"] = score
		result["alerts"] = alerts
	} else {
		result["metrics_error"] = err.Error()
	}
	return result, nil
}

func pcOSWindowsDisasterRecovery() (interface{}, error) {
	base, _ := pcOSDisasterRecovery()
	return map[string]interface{}{
		"module":             "disaster_recovery",
		"generated_utc":      time.Now().UTC().Format(time.RFC3339),
		"mode":               "plan_and_verify",
		"read_only":          true,
		"automatic_restore":  false,
		"plan":               base,
		"disks":              pcOSCapture("disks", diskInventory),
		"windows_update":     pcOSCapture("windows_update", windowsUpdateStatus),
		"security":           pcOSCapture("security", securityStatus),
		"network":            pcOSCapture("network", networkList),
		"services":           pcOSCapture("services", serviceList),
	}, nil
}

func pcOSWindowsOverview() (interface{}, error) {
	modules, _ := pcOSModules()
	guardian, _ := pcOSWindowsGuardian()
	network, _ := pcOSNetwork()
	updateChain, _ := pcOSUpdateChain()
	zeroUI, _ := pcOSZeroUI()
	return map[string]interface{}{
		"pc_os_version": pcOSVersion,
		"generated_utc": time.Now().UTC().Format(time.RFC3339),
		"safe_mode": true,
		"destructive_action_executed": false,
		"modules": modules,
		"guardian": guardian,
		"network": network,
		"update_chain": updateChain,
		"zero_ui": zeroUI,
	}, nil
}
