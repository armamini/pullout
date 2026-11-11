//go:build !unix

package main

import "os/exec"

// isolate is a no-op where sessions and process groups are unavailable;
// exec.CommandContext still kills the tool itself on cancellation.
func isolate(*exec.Cmd) {}
