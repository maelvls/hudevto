// Copied over from https://github.com/tetrafolium/luci-go/blob/4a11b793d/common/system/pager/pager.go.

// Copyright 2019 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package pager implements paging using commands "less" or "more",
// depending on availability.
package pager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func done(err error) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "running pager: %s\n", err)
		return 1
	}
	return 0
}

// Main implements paging using commands "less" or "more" if they are available.
// If os.Stdout is not terminal or less/more are not available in $PATH, Main
// calls fn with out set to os.Stdout and returns its exit code. Otherwise
// creates a pager subprocess, directs its stdout to os.Stdout and calls fn with
// out set to pager stdin. fn's context is canceled if the user quits pager.
//
// If fn returns non-zero exit code before pager exits, Main returns that exit
// code. Otherwise Main returns pager's exit code.
// It is a race between the user hitting q and fn failing.
//
// Example:
//
//	func main() int {
//		return Main(context.Background(), func(ctx context.Context, out io.WriteCloser) int {
//			for i := 0; i < 100000 && ctx.Err() == nil; i++ {
//				fmt.Fprintln(out, i)
//			}
//			return 0
//		})
//	}
func Main(ctx context.Context, fn func(ctx context.Context, out io.WriteCloser) int) int {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fn(ctx, os.Stdout)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigC := make(chan os.Signal, 1)
	var cmd *exec.Cmd
	if lessPath, _ := exec.LookPath("less"); lessPath != "" {
		cmd = exec.Command(lessPath, "-FXr")

		// Swallow interrupts. Less is supposed to be quit by pressing q.
		// In particular, it does not respond to Ctrl+C.
		signal.Notify(sigC, os.Interrupt, os.Kill)
		defer signal.Stop(sigC)
	} else if morePath, _ := exec.LookPath("more"); morePath != "" {
		moreCtx, cancelMore := context.WithCancel(ctx)
		cmd = exec.CommandContext(moreCtx, morePath)

		// Forward Ctrl+C to more.
		signal.Notify(sigC, os.Interrupt, os.Kill)
		go func() {
			for range sigC {
				cancelMore()
			}
		}()
	} else {
		// A pager program is not available.
		return fn(ctx, os.Stdout)
	}
	defer signal.Stop(sigC)

	cmd.Stdout = os.Stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return done(fmt.Errorf("piping stdin to pager: %w", err))
	}

	if err := cmd.Start(); err != nil {
		return done(fmt.Errorf("launching pager: %w", err))
	}

	// Listen to both fn and pager.
	exitCodeC := make(chan int, 2)
	go func() {
		if exitCode := fn(ctx, stdin); exitCode != 0 {
			exitCodeC <- exitCode
		}
		// Let the pager know that this is the end.
		stdin.Close()
	}()

	go func() {
		if exitCode, ok := Get(cmd.Wait()); ok {
			exitCodeC <- exitCode
		} else {
			exitCodeC <- done(err)
		}
		cancel()
	}()

	return <-exitCodeC
}

// Get returns the process process exit return code given an error returned by
// exec.Cmd's Wait or Run methods. If no exit code is present, Get will return
// false.
func Get(err error) (int, bool) {
	err = errors.Unwrap(err)
	if err == nil {
		return 0, true
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.Sys().(syscall.WaitStatus).ExitStatus(), true
	}
	return 0, false
}
