//go:build !linux

package main

import "os"

// The service is Linux-only; other hosts can exercise headless unit tests.
func isTerminal(*os.File) bool { return false }
