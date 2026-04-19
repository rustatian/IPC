//go:build linux

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rustatian/ipc/shm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// semanticName returns a slash-prefixed POSIX shm name unique to the test.
func posixShmName(t testing.TB) string {
	t.Helper()
	// Replace '/' in subtest names since POSIX shm names may not contain
	// them after the leading slash.
	return "/ipc-" + strings.ReplaceAll(t.Name(), "/", "_")
}

func TestNewSharedMemoryPosix(t *testing.T) {
	name := posixShmName(t)
	seg, err := shm.NewSharedMemoryPosix(name, 1024, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg.Detach()
		_ = seg.Remove()
	})

	require.NoError(t, seg.Write([]byte(testData)))

	buf := make([]byte, len(testData))
	n, err := seg.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, string(buf))
}

// POSIX Remove must unlink the backing file, and a fresh create of the
// same name must return a zeroed segment (not leftover data).
func TestPosixRemoveUnlinksAndResets(t *testing.T) {
	name := posixShmName(t)
	path := filepath.Join(shm.PosixShmDir, name[1:]) // strip leading '/'

	seg, err := shm.NewSharedMemoryPosix(name, 64, 0)
	require.NoError(t, err)

	require.NoError(t, seg.Write([]byte("leftover-data")))
	require.NoError(t, seg.Detach())
	require.NoError(t, seg.Remove())

	// File should be gone.
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "Remove should have unlinked %s", path)

	// A fresh segment of the same name must be zero-initialized.
	fresh, err := shm.NewSharedMemoryPosix(name, 64, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fresh.Detach()
		_ = fresh.Remove()
	})

	buf := make([]byte, 64)
	_, err = fresh.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, make([]byte, 64), buf, "fresh segment should be zeroed")
}

// Path-traversal and invalid-name inputs are rejected before reaching
// the filesystem.
func TestPosixNameValidation(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{"empty", ""},
		{"no leading slash", "foo"},
		{"only slash", "/"},
		{"internal slash", "/foo/bar"},
		{"dotdot", "/.."},
		{"dotdot embedded", "/foo..bar"},
		{"null byte", "/foo\x00bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := shm.NewSharedMemoryPosix(tc.arg, 64, 0)
			assert.Error(t, err, "name %q should be rejected", tc.arg)
		})
	}
}

// POSIX segments have no SysV ID, so ID() returns (0, false).
func TestPosixIDReturnsFalse(t *testing.T) {
	name := posixShmName(t)
	seg, err := shm.NewSharedMemoryPosix(name, 64, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg.Detach()
		_ = seg.Remove()
	})

	id, ok := seg.ID()
	assert.False(t, ok, "POSIX segment must not claim to have a SysV ID")
	assert.Zero(t, id)
}
