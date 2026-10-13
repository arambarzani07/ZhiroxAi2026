package main

import (
	"testing"
	"time"
)

func TestZeroTouchNeverRestartsWhenEndpointUnreachable(t *testing.T) {
	d := pcOSZeroTouchEvaluate(false, false, 30*time.Minute, 99, 30*time.Minute, 0)
	if d.Restart || d.ConsecutiveStale != 0 || d.Reason != "watchdog_endpoint_unreachable" {
		t.Fatalf("unexpected decision: %#v", d)
	}
}

func TestZeroTouchNeverRestartsOnAuthenticationFailure(t *testing.T) {
	d := pcOSZeroTouchEvaluate(true, false, 30*time.Minute, 99, 30*time.Minute, 0)
	if d.Restart || d.ConsecutiveStale != 0 || d.Reason != "watchdog_authentication_failed" {
		t.Fatalf("unexpected decision: %#v", d)
	}
}

func TestZeroTouchFreshHeartbeatResetsStaleCount(t *testing.T) {
	d := pcOSZeroTouchEvaluate(true, true, 30*time.Second, 2, 30*time.Minute, 0)
	if d.Restart || d.ConsecutiveStale != 0 || d.Reason != "heartbeat_fresh" {
		t.Fatalf("unexpected decision: %#v", d)
	}
}

func TestZeroTouchRequiresThreeStaleConfirmations(t *testing.T) {
	d1 := pcOSZeroTouchEvaluate(true, true, 3*time.Minute, 0, 30*time.Minute, 0)
	if d1.Restart || d1.ConsecutiveStale != 1 {
		t.Fatalf("first stale observation: %#v", d1)
	}
	d2 := pcOSZeroTouchEvaluate(true, true, 3*time.Minute, d1.ConsecutiveStale, 30*time.Minute, 0)
	if d2.Restart || d2.ConsecutiveStale != 2 {
		t.Fatalf("second stale observation: %#v", d2)
	}
	d3 := pcOSZeroTouchEvaluate(true, true, 3*time.Minute, d2.ConsecutiveStale, 30*time.Minute, 0)
	if !d3.Restart || d3.Reason != "heartbeat_stale_confirmed" {
		t.Fatalf("third stale observation: %#v", d3)
	}
}

func TestZeroTouchHonorsCooldown(t *testing.T) {
	d := pcOSZeroTouchEvaluate(true, true, 5*time.Minute, 2, 2*time.Minute, 0)
	if d.Restart || d.Reason != "restart_cooldown" {
		t.Fatalf("unexpected decision: %#v", d)
	}
}

func TestZeroTouchHonorsRestartBudget(t *testing.T) {
	d := pcOSZeroTouchEvaluate(true, true, 5*time.Minute, 2, 30*time.Minute, pcOSZeroTouchMaxRestartsPerWindow)
	if d.Restart || d.Reason != "restart_budget_exhausted" {
		t.Fatalf("unexpected decision: %#v", d)
	}
}
