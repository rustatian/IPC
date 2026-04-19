//go:build unix

package tests

import (
	"errors"
	"hash/fnv"
	"syscall"
	"testing"

	"github.com/rustatian/ipc/apis"
	"github.com/rustatian/ipc/shm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testData = "hello my dear friend"

// interfaceConformance is a compile-time assertion: SharedMemorySegment
// must implement the apis.SharedMemory interface. If the interface or the
// struct drift out of sync, this line fails to compile.
var _ apis.SharedMemory = (*shm.SharedMemorySegment)(nil)

// testKey produces a deterministic per-test SysV key unlikely to collide
// with other tests or running processes. Folding t.Name() keeps it stable
// across runs of the same test.
func testKey(t testing.TB) int {
	t.Helper()
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	return int(h.Sum32() & 0x7FFFFFFF)
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

	require.NoError(t, seg1.Clear())
	require.NoError(t, seg1.Write([]byte(testData)))
	require.NoError(t, seg1.Detach())

	id, ok := seg1.ID()
	require.True(t, ok, "SysV segment should expose an ID")

	seg2, err := shm.AttachToShmSegment(id, 1024)
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

func TestIpcExclRejectsExistingSegment(t *testing.T) {
	key := testKey(t)

	first, err := shm.NewSharedMemorySegment(key, 64, shm.SIrusr|shm.SIwusr, shm.IpcCreat|shm.IpcExcl)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = first.Detach()
		_ = first.Remove()
	})

	_, err = shm.NewSharedMemorySegment(key, 64, shm.SIrusr|shm.SIwusr, shm.IpcCreat|shm.IpcExcl)
	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.EEXIST)

	second, err := shm.NewSharedMemorySegment(key, 64, shm.SIrusr|shm.SIwusr, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = second.Detach()
	})

	firstID, _ := first.ID()
	secondID, _ := second.ID()
	assert.Equal(t, firstID, secondID, "both handles should reference the same segment")

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

// Write and Clear must error after Detach, and Detach itself must be
// idempotent.
func TestWriteClearAfterDetach(t *testing.T) {
	key := testKey(t)
	seg, err := shm.NewSharedMemorySegment(key, 16, shm.SIrusr|shm.SIwusr, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg.Remove()
	})

	require.NoError(t, seg.Detach())

	assert.Error(t, seg.Write([]byte("x")), "Write after Detach must error")
	assert.Error(t, seg.Clear(), "Clear after Detach must error")

	// Detach is idempotent.
	require.NoError(t, seg.Detach())
}

// Size and ID getters return the expected values for a SysV segment.
func TestSizeAndID(t *testing.T) {
	key := testKey(t)
	seg, err := shm.NewSharedMemorySegment(key, 256, shm.SIrusr|shm.SIwusr, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = seg.Detach()
		_ = seg.Remove()
	})

	assert.Equal(t, uint(256), seg.Size())

	id, ok := seg.ID()
	assert.True(t, ok, "SysV segment must have an ID")
	assert.NotZero(t, id)
}

// Size and validation rejection paths.
func TestInputValidation(t *testing.T) {
	t.Run("zero size rejected", func(t *testing.T) {
		_, err := shm.NewSharedMemorySegment(testKey(t), 0, 0600, shm.IpcCreat)
		assert.Error(t, err)
	})
	t.Run("zero size on Attach rejected", func(t *testing.T) {
		_, err := shm.AttachToShmSegment(1, 0)
		assert.Error(t, err)
	})
}

// Remove is idempotent — the second call is a no-op.
func TestRemoveIdempotent(t *testing.T) {
	key := testKey(t)
	seg, err := shm.NewSharedMemorySegment(key, 16, shm.SIrusr|shm.SIwusr, shm.IpcCreat)
	require.NoError(t, err)
	t.Cleanup(func() { _ = seg.Detach() })

	require.NoError(t, seg.Remove(), "first Remove should succeed")
	require.NoError(t, seg.Remove(), "second Remove should be a no-op")
}

func BenchmarkAttachToShmSegment_READ(b *testing.B) {
	bigJSONLen := len(BigJSON)
	key := testKey(b)
	seg1, err := shm.NewSharedMemorySegment(key, uint(bigJSONLen), shm.SIrusr|shm.SIwusr|shm.SIrgrp|shm.SIwgrp, shm.IpcCreat)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = seg1.Remove()
	})

	_ = seg1.Clear()
	if err := seg1.Write([]byte(testData)); err != nil {
		b.Fatal(err)
	}
	if err := seg1.Detach(); err != nil {
		b.Fatal(err)
	}

	id, _ := seg1.ID()
	seg2, err := shm.AttachToShmSegment(id, uint(bigJSONLen))
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
	key := testKey(b)
	seg, err := shm.NewSharedMemorySegment(key, uint(bigJSONLen), shm.SIrusr|shm.SIwusr|shm.SIrgrp|shm.SIwgrp, shm.IpcCreat)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = seg.Detach()
		_ = seg.Remove()
	})

	_ = seg.Clear()
	payload := []byte(testData)
	b.ReportAllocs()

	for b.Loop() {
		if err := seg.Write(payload); err != nil {
			b.Fatal(err)
		}
		_ = seg.Clear()
	}
}
