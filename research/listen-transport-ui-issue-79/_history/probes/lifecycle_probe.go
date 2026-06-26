// PROBE to settle claims 4, 7, 8 — NOT an implementation. Throwaway.
// Models the cmd/narrate controller lifecycle:
//   - MkdirTemp once (claim 8)
//   - spawn a child (stands in for afplay), Kill async + Wait to reap (claim 7)
//   - signal handler does RemoveAll + kill-child on SIGINT (claim 4)
//
// Modes:
//
//	handler  : install signal handler, spawn child, wait for SIGINT, clean, exit 0
//	nohandler: rely on a defer RemoveAll, spawn child, wait for SIGINT (defer skipped)
//	reap     : demonstrate Kill is async -> Wait reaps; second Start has no overlap
//	removeall: demonstrate RemoveAll idempotent on missing path
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	mode := "handler"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	switch mode {
	case "handler":
		runHandler()
	case "nohandler":
		runNoHandler()
	case "reap":
		runReap()
	case "removeall":
		runRemoveAll()
	default:
		fmt.Println("unknown mode")
		os.Exit(2)
	}
}

func mkTemp() string {
	d, err := os.MkdirTemp("", "narrate-session-*")
	if err != nil {
		panic(err)
	}
	fmt.Printf("TEMPDIR=%s\n", d)
	return d
}

func spawnChild() *exec.Cmd {
	// stands in for afplay: long-lived child we must reap
	c := exec.Command("sleep", "120")
	if err := c.Start(); err != nil {
		panic(err)
	}
	fmt.Printf("CHILD_PID=%d\n", c.Process.Pid)
	return c
}

// claim 4 + 8 + 7: signal handler cleans temp dir and kills+reaps child on SIGINT.
func runHandler() {
	dir := mkTemp()
	child := spawnChild()
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("READY") // parent prints this then blocks on signal
	<-sigc
	_ = child.Process.Kill() // async
	_, _ = child.Process.Wait()
	_ = os.RemoveAll(dir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Println("CLEANED=yes")
	} else {
		fmt.Println("CLEANED=no")
	}
	fmt.Println("HANDLER_EXIT")
	os.Exit(0)
}

// claim 4 (negative): a bare defer RemoveAll is SKIPPED when SIGINT terminates the
// process by default (no handler installed). Demonstrates the cleanup hazard.
func runNoHandler() {
	dir := mkTemp()
	_ = spawnChild()
	defer func() {
		_ = os.RemoveAll(dir)
		fmt.Println("DEFER_RAN") // should NOT appear when killed by SIGINT
	}()
	fmt.Println("READY")
	time.Sleep(120 * time.Second) // wait to be SIGINT'd; default action terminates, defer skipped
}

// claim 7: Kill is async; Wait reaps; reaping before next Start means no overlap.
func runReap() {
	c1 := exec.Command("sleep", "120")
	_ = c1.Start()
	pid1 := c1.Process.Pid
	_ = c1.Process.Kill() // async — process may still be alive immediately after
	state, _ := c1.Process.Wait()
	fmt.Printf("REAPED pid=%d exited=%v\n", pid1, state.Exited())
	// only after reap do we Start the next — no two sleeps overlap
	c2 := exec.Command("sleep", "1")
	_ = c2.Start()
	pid2 := c2.Process.Pid
	_ = c2.Wait()
	fmt.Printf("SECOND_DONE pid=%d (pid1=%d reaped first)\n", pid2, pid1)
}

// claim 8: RemoveAll is idempotent — returns nil on an already-missing path.
func runRemoveAll() {
	dir := mkTemp()
	err1 := os.RemoveAll(dir)
	err2 := os.RemoveAll(dir) // second call on now-missing path
	err3 := os.RemoveAll(dir + "/does-not-exist-subpath")
	fmt.Printf("RemoveAll#1 err=%v\n", err1)
	fmt.Printf("RemoveAll#2(missing) err=%v\n", err2)
	fmt.Printf("RemoveAll#3(never-existed) err=%v\n", err3)
}
