// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// notifySigWinch listens for SIGWINCH signals and updates the terminal size
// for the given SSH session.
func notifySigWinch(fd int, session *ssh.Session) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			w, h, err := term.GetSize(fd)
			if err != nil {
				continue
			}
			session.WindowChange(h, w)
		}
	}()
}
