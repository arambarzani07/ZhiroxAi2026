//go:build windows && zhiroxtest

package main

// These stubs exist only for the isolated Windows updater integration test.
// Production builds do not set the zhiroxtest build tag and therefore never
// include this file.
func sendHeartbeat() error { return nil }
func pcOSActiveRoot() string { return "" }
