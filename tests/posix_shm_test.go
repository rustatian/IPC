//go:build unix

package tests

import (
	"errors"
	"hash/fnv"
	"os"
	"syscall"
	"testing"

	"github.com/rustatian/ipc/shm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testData = "hello my dear friend"

// testKey produces a deterministic per-test SysV key unlikely to collide with
// other tests or running processes. Folding t.Name() into 31 bits keeps it a
// positive int and stable across runs of the same test.
func testKey(t *testing.T) int {
	t.Helper()
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	return int(h.Sum32() & 0x7FFFFFFF)
}

func TestNewSharedMemoryPosix(t *testing.T) {
	if _, err := os.Stat(shm.PosixShmDir); err != nil {
		t.Skipf("%s is not available on this platform: %v", shm.PosixShmDir, err)
	}
	seg, err := shm.NewSharedMemoryPosix("ipc-test-posix", 1024, 0)
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

func TestNewSharedMemorySegment(t *testing.T) {
	key := testKey(t)
	seg1, err := shm.NewSharedMemorySegment(key, 1024, shm.SIrusr|shm.SIwusr|shm.SIrgrp|shm.SIwgrp, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg1.Remove()
	})

	require.NoError(t, seg1.Write([]byte(testData)))
	require.NoError(t, seg1.Detach())

	seg2, err := shm.NewSharedMemorySegment(key, 1024, 0, shm.Rdonly)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg2.Detach()
	})

	buf := make([]byte, len(testData))
	_, err = seg2.Read(buf)
	require.NoError(t, err)

	assert.Equal(t, []byte(testData), buf)
}

func TestAttachToShmSegment(t *testing.T) {
	key := testKey(t)
	seg1, err := shm.NewSharedMemorySegment(key, 1024, shm.SIrusr|shm.SIwusr|shm.SIrgrp|shm.SIwgrp, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg1.Remove()
	})

	seg1.Clear()
	require.NoError(t, seg1.Write([]byte(testData)))
	require.NoError(t, seg1.Detach())

	seg2, err := shm.AttachToShmSegment(seg1.ID(), 1024)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg2.Detach()
	})

	buf := make([]byte, len(testData))
	_, err = seg2.Read(buf)
	require.NoError(t, err)

	assert.Equal(t, []byte(testData), buf)
}

func TestWriteRejectsOversizedInput(t *testing.T) {
	key := testKey(t)
	seg, err := shm.NewSharedMemorySegment(key, 16, shm.SIrusr|shm.SIwusr, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg.Detach()
		_ = seg.Remove()
	})

	err = seg.Write(make([]byte, 32))
	assert.Error(t, err)
}

// TestIpcExclRejectsExistingSegment verifies the exclusive-creation contract:
// with IpcCreat|IpcExcl, a second attempt to create a segment that already
// exists fails with EEXIST. This is how callers detect leftover segments
// from a crashed previous run or arbitrate which process "wins" creation
// in a multi-process startup race.
func TestIpcExclRejectsExistingSegment(t *testing.T) {
	key := testKey(t)

	first, err := shm.NewSharedMemorySegment(key, 64, shm.SIrusr|shm.SIwusr, shm.IpcCreat|shm.IpcExcl)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = first.Detach()
		_ = first.Remove()
	})

	// Second exclusive create on the same key must fail with EEXIST.
	_, err = shm.NewSharedMemorySegment(key, 64, shm.SIrusr|shm.SIwusr, shm.IpcCreat|shm.IpcExcl)
	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.EEXIST)

	// Plain IpcCreat (without IpcExcl) should still succeed — it attaches
	// to the existing segment rather than failing.
	second, err := shm.NewSharedMemorySegment(key, 64, shm.SIrusr|shm.SIwusr, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = second.Detach()
	})
	assert.Equal(t, first.ID(), second.ID(), "both handles should reference the same segment")

	// And ensure the error chain is walkable for callers using errors.Is.
	_, err = shm.NewSharedMemorySegment(key, 64, shm.SIrusr|shm.SIwusr, shm.IpcCreat|shm.IpcExcl)
	assert.True(t, errors.Is(err, syscall.EEXIST))
}

func TestReadAfterDetach(t *testing.T) {
	key := testKey(t)
	seg, err := shm.NewSharedMemorySegment(key, 16, shm.SIrusr|shm.SIwusr, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg.Remove()
	})

	require.NoError(t, seg.Detach())

	_, err = seg.Read(make([]byte, 1))
	assert.Error(t, err)
}

func BenchmarkAttachToShmSegment_READ(b *testing.B) {
	bigJSONLen := len(BigJSON)
	key := 0x10
	seg1, err := shm.NewSharedMemorySegment(key, uint(bigJSONLen), shm.SIrusr|shm.SIwusr|shm.SIrgrp|shm.SIwgrp, shm.IpcCreat)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = seg1.Remove()
	})

	seg1.Clear()
	if err := seg1.Write([]byte(testData)); err != nil {
		b.Fatal(err)
	}
	if err := seg1.Detach(); err != nil {
		b.Fatal(err)
	}

	seg2, err := shm.AttachToShmSegment(seg1.ID(), uint(bigJSONLen))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = seg2.Detach()
	})

	buf := make([]byte, bigJSONLen)
	b.ReportAllocs()

	for b.Loop() {
		if _, err := seg2.Read(buf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAttachToShmSegment_WRITE(b *testing.B) {
	bigJSONLen := len(BigJSON)
	key := 0x20
	seg, err := shm.NewSharedMemorySegment(key, uint(bigJSONLen), shm.SIrusr|shm.SIwusr|shm.SIrgrp|shm.SIwgrp, shm.IpcCreat)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = seg.Detach()
		_ = seg.Remove()
	})

	seg.Clear()
	payload := []byte(testData)
	b.ReportAllocs()

	for b.Loop() {
		if err := seg.Write(payload); err != nil {
			b.Fatal(err)
		}
		seg.Clear()
	}
}
