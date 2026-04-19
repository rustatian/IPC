//go:build linux || darwin

package tests

import (
	"errors"
	"hash/fnv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rustatian/ipc/semaphore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// semName produces a slash-prefixed name unique per test. FNV-hashing
// t.Name() keeps it short (POSIX name length is capped at ~30 chars on
// some systems) and stable across test runs.
func semName(t *testing.T) string {
	t.Helper()
	h := fnv.New64a()
	_, _ = h.Write([]byte(t.Name()))
	return "/ipc-" + hex(h.Sum64())
}

func hex(v uint64) string {
	const digits = "0123456789abcdef"
	buf := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		buf[i] = digits[v&0xF]
		v >>= 4
	}
	return string(buf)
}

func TestSemaphore_PostWaitAcrossGoroutines(t *testing.T) {
	name := semName(t)

	s, err := semaphore.NewSemaphore(name, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
		_ = s.Unlink()
	})

	var wg sync.WaitGroup
	wg.Go(func() {
		peer, err := semaphore.OpenSemaphore(name)
		if !assert.NoError(t, err) {
			return
		}
		defer func() { _ = peer.Close() }()

		time.Sleep(100 * time.Millisecond)
		assert.NoError(t, peer.Post())
	})

	require.NoError(t, s.Wait())
	wg.Wait()
}

func TestSemaphore_TryWait(t *testing.T) {
	name := semName(t)

	s, err := semaphore.NewSemaphore(name, 0, 1)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
		_ = s.Unlink()
	})

	// counter 1 → 0
	require.NoError(t, s.TryWait())

	// counter is zero; TryWait must return EAGAIN without blocking
	err = s.TryWait()
	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.EAGAIN)

	// Post then TryWait succeeds again
	require.NoError(t, s.Post())
	require.NoError(t, s.TryWait())
}

func TestSemaphore_ExclusiveCreation(t *testing.T) {
	name := semName(t)

	first, err := semaphore.NewSemaphore(name, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = first.Close()
		_ = first.Unlink()
	})

	// Second NewSemaphore on the same name must fail with EEXIST.
	_, err = semaphore.NewSemaphore(name, 0, 0)
	require.Error(t, err)
	assert.True(t, errors.Is(err, syscall.EEXIST))

	// OpenSemaphore attaches without creation.
	second, err := semaphore.OpenSemaphore(name)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
}

func TestSemaphore_OpenOrCreateIsIdempotent(t *testing.T) {
	name := semName(t)

	first, err := semaphore.OpenOrCreateSemaphore(name, 0, 3)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = first.Close()
		_ = first.Unlink()
	})

	// Second call attaches to the existing semaphore; the initial arg is
	// ignored on an already-existing object.
	second, err := semaphore.OpenOrCreateSemaphore(name, 0, 99)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
}

func TestSemaphore_Name(t *testing.T) {
	// Name without slash prefix gets normalized.
	raw := semName(t)[1:] // strip the '/'
	s, err := semaphore.NewSemaphore(raw, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
		_ = s.Unlink()
	})

	assert.Equal(t, "/"+raw, s.Name())
}
