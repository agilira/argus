// setup_shape4_kubernetes_test.go - shape 4: a ConfigMap directory plus the environment
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus_test

import (
	"path/filepath"
	"testing"

	"github.com/agilira/argus"
)

// TestShape4_Kubernetes mounts a ConfigMap as a directory: several files, each
// one a fragment, merged into a single key space, with the environment on top
// for the values the deployment injects.
//
// The directory is watched as a directory, so a key that appears in a new file
// after a ConfigMap update is seen without a restart.
func TestShape4_Kubernetes(t *testing.T) {
	dir := t.TempDir()
	mount := filepath.Join(dir, "etc", "config")
	writeFile(t, filepath.Join(mount, "app.json"), `{"port": 8080, "workers": 4}`)
	writeFile(t, filepath.Join(mount, "features.json"), `{"tracing": true}`)

	t.Setenv("SVC_WORKERS", "16")

	// The one line.
	settings, err := argus.Setup("svc").Dir(mount).Env("SVC_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	// Both files contribute to one key space.
	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d, want 8080", got)
	}
	if !settings.GetBool("tracing") {
		t.Error("tracing = false, want true from the second file")
	}
	// And the environment still outranks the mount.
	if got := settings.GetInt("workers"); got != 16 {
		t.Errorf("workers = %d, want 16 from the environment", got)
	}
}
