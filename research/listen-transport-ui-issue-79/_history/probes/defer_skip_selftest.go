//go:build ignore

// Self-contained negative test for claim 4: prove that the default SIGINT
// disposition terminates the program WITHOUT running deferred cleanup.
// The program raises SIGINT to itself, so no external signal plumbing is needed.
package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func main() {
	dir, _ := os.MkdirTemp("", "narrate-selftest-*")
	fmt.Printf("TEMPDIR=%s\n", dir)
	// A bare defer that "would" clean up — the whole point is it gets SKIPPED.
	defer func() {
		_ = os.RemoveAll(dir)
		fmt.Println("DEFER_RAN") // must NOT print
	}()
	fmt.Println("RAISING_SIGINT")
	// Default Go behavior: an uncaught SIGINT terminates the process; deferred
	// funcs do NOT run (process dies, stack does not unwind).
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	time.Sleep(2 * time.Second) // give the signal time to land
	fmt.Println("REACHED_END") // must NOT print either
}
