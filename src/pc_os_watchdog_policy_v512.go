package main

import "time"

const (
	pcOSZeroTouchCheckInterval       = 20 * time.Second
	pcOSZeroTouchHeartbeatStaleAfter = 2 * time.Minute
	pcOSZeroTouchConfirmations       = 3
	pcOSZeroTouchRestartCooldown     = 10 * time.Minute
	pcOSZeroTouchRestartWindow       = 1 * time.Hour
	pcOSZeroTouchMaxRestartsPerWindow = 3
)

type pcOSZeroTouchDecision struct {
	Restart          bool
	ConsecutiveStale int
	Reason           string
}

// pcOSZeroTouchEvaluate is intentionally deterministic and side-effect free.
// A restart is permitted only when the watchdog endpoint is reachable and
// authenticated, the server-observed Supervisor heartbeat is stale for three
// consecutive observations, the restart cooldown has elapsed, and the bounded
// hourly restart budget is still available.
func pcOSZeroTouchEvaluate(
	endpointReachable bool,
	authenticated bool,
	heartbeatAge time.Duration,
	consecutiveStale int,
	timeSinceLastRestart time.Duration,
	restartsInWindow int,
) pcOSZeroTouchDecision {
	if !endpointReachable {
		return pcOSZeroTouchDecision{Reason: "watchdog_endpoint_unreachable"}
	}
	if !authenticated {
		return pcOSZeroTouchDecision{Reason: "watchdog_authentication_failed"}
	}
	if heartbeatAge < 0 {
		return pcOSZeroTouchDecision{Reason: "heartbeat_age_invalid"}
	}
	if heartbeatAge < pcOSZeroTouchHeartbeatStaleAfter {
		return pcOSZeroTouchDecision{Reason: "heartbeat_fresh"}
	}

	next := consecutiveStale + 1
	if next < pcOSZeroTouchConfirmations {
		return pcOSZeroTouchDecision{ConsecutiveStale: next, Reason: "heartbeat_stale_confirming"}
	}
	if restartsInWindow >= pcOSZeroTouchMaxRestartsPerWindow {
		return pcOSZeroTouchDecision{ConsecutiveStale: next, Reason: "restart_budget_exhausted"}
	}
	if timeSinceLastRestart >= 0 && timeSinceLastRestart < pcOSZeroTouchRestartCooldown {
		return pcOSZeroTouchDecision{ConsecutiveStale: next, Reason: "restart_cooldown"}
	}

	return pcOSZeroTouchDecision{Restart: true, Reason: "heartbeat_stale_confirmed"}
}
