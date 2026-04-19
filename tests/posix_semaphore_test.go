//go:build linux || darwin

package tests

import (
	"errors"
	"hash/fnv"
	"sync"
	"syscall"
	"testing"

	"github.com/rustatian/ipc/semaphore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// semName produces a slash-prefixed name unique per test. FNV-hashing
// t.Name() keeps it short (POSIX name length is capped at ~30 chars on
// some systems) and stable across test runs.
func semName(t testing.TB) string {
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

// Post/Wait handoff uses a channel barrier rather than time.Sleep — the
// producer posts only after the consumer has parked, eliminating
// timing-based flakiness under CI load.
func TestSemaphore_PostWaitAcrossGoroutines(t *testing.T) {
	name := semName(t)

	s, err := semaphore.NewSemaphore(name, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
		_ = s.Unlink()
	})

	ready := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		peer, err := semaphore.OpenSemaphore(name)
		if !assert.NoError(t, err) {
			close(ready)
			return
		}
		defer func() { _ = peer.Close() }()

		close(ready) // signal to the main goroutine it can now Wait
		assert.NoError(t, peer.Post())
	})

	<-ready
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

	require.NoError(t, s.TryWait())

	err = s.TryWait()
	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.EAGAIN)

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

	_, err = semaphore.NewSemaphore(name, 0, 0)
	require.Error(t, err)
	assert.True(t, errors.Is(err, syscall.EEXIST))

	second, err := semaphore.OpenSemaphore(name)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
}

// OpenOrCreate must honor initial on actual creation, and a later attach
// must read the counter set by the first creator (not the later initial
// arg, which must be ignored).
func TestSemaphore_OpenOrCreateHonorsInitialOnCreation(t *testing.T) {
	name := semName(t)

	first, err := semaphore.OpenOrCreateSemaphore(name, 0, 3)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = first.Close()
		_ = first.Unlink()
	})

	// A later caller passes a different initial — it must be ignored
	// because the semaphore already exists. A "leaky" implementation
	// would overwrite the counter and break the first caller.
	second, err := semaphore.OpenOrCreateSemaphore(name, 0, 99)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	// Counter should be 3 (from first) — verify by draining.
	for i := range 3 {
		require.NoError(t, second.TryWait(), "expected 3 permits, failed at %d", i)
	}
	// Next TryWait must fail since we've exhausted the counter.
	err = second.TryWait()
	assert.ErrorIs(t, err, syscall.EAGAIN)
}

func TestSemaphore_Name(t *testing.T) {
	raw := semName(t)[1:] // strip the '/'
	s, err := semaphore.NewSemaphore(raw, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
		_ = s.Unlink()
	})

	assert.Equal(t, "/"+raw, s.Name())
}

// After Close, Post/Wait/TryWait must return "semaphore is closed". This
// is load-bearing for cross-platform parity — both Linux and darwin must
// behave the same after Close.
func TestSemaphore_ClosedRejectsOps(t *testing.T) {
	name := semName(t)
	s, err := semaphore.NewSemaphore(name, 0, 1)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Unlink()
	})

	require.NoError(t, s.Close())

	assert.Error(t, s.Post())
	assert.Error(t, s.Wait())
	assert.Error(t, s.TryWait())

	// Close is idempotent.
	assert.NoError(t, s.Close())
}

// Unlink is idempotent — a second call returns nil on both platforms.
func TestSemaphore_UnlinkIdempotent(t *testing.T) {
	name := semName(t)
	s, err := semaphore.NewSemaphore(name, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	require.NoError(t, s.Unlink(), "first Unlink should succeed")
	require.NoError(t, s.Unlink(), "second Unlink should be a no-op")
}

// OpenSemaphore on a nonexistent name returns ENOENT through errors.Is.
// This is the "does it exist?" primary-bootstrap check.
func TestSemaphore_OpenNonexistentReturnsENOENT(t *testing.T) {
	_, err := semaphore.OpenSemaphore(semName(t))
	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.ENOENT)
}

// Input validation: empty name is rejected.
func TestSemaphore_EmptyNameRejected(t *testing.T) {
	_, err := semaphore.NewSemaphore("", 0, 0)
	assert.Error(t, err)

	_, err = semaphore.OpenSemaphore("")
	assert.Error(t, err)
}
