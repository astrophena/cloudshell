// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build !unix

package main

import "golang.org/x/crypto/ssh"

// notifySigWinch is a no-op on non-Unix systems, as they don't have syscall.SIGWINCH.
func notifySigWinch(_ int, _ *ssh.Session) {}
