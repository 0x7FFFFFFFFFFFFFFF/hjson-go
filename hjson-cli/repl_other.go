//go:build !windows

package main

import (
	"bufio"
	"io"
	"os"
	"os/signal"
	"strings"
)

// replReadLoop reads lines from stdin. Each Ctrl+C (SIGINT) submits the
// buffered input for conversion and keeps the REPL running. EOF (Ctrl+D) quits.
func replReadLoop(submit chan<- string, done chan<- struct{}) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	lines := make(chan string, 1024)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if line != "" {
					lines <- strings.TrimRight(line, "\r\n")
				}
				if err == io.EOF {
					close(lines)
					return
				}
				// Transient error (e.g. EINTR after a signal); keep reading.
				continue
			}
			lines <- strings.TrimRight(line, "\r\n")
		}
	}()

	var buf []string
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				done <- struct{}{}
				return
			}
			buf = append(buf, line)
		case <-sigCh:
			buf = drainLines(lines, buf)
			submit <- strings.Join(buf, "\n")
			buf = buf[:0]
		}
	}
}

// drainLines appends any immediately-available lines to buf without blocking.
func drainLines(lines <-chan string, buf []string) []string {
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return buf
			}
			buf = append(buf, line)
		default:
			return buf
		}
	}
}

// restoreConsole is a no-op on non-Windows platforms.
func restoreConsole() {}
