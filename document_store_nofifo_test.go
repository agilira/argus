//go:build windows

// document_store_nofifo_test.go - the FIFO helper's stub on Windows
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import "testing"

func mkfifo(t *testing.T, _ string) {
	t.Helper()
	t.Skip("no FIFOs on Windows")
}
