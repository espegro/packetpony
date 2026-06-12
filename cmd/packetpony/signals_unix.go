//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func notifySignals(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
}

func isReloadSignal(sig os.Signal) bool {
	return sig == syscall.SIGHUP
}
