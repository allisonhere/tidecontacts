//go:build !windows

package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"os/signal"
	"syscall"
)

// Bubble Tea handles INT/TERM itself; treat terminal hangup as an orderly quit.
func handleHangup(p *tea.Program) func() {
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		select {
		case <-ch:
			p.Send(tea.QuitMsg{})
		case <-done:
		}
	}()
	return func() { signal.Stop(ch); close(done) }
}
