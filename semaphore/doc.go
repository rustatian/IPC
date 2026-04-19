// Package semaphore provides named counting semaphores for interprocess
// coordination via raw system calls — no cgo.
//
// The API is POSIX-flavored: semaphores are identified by a slash-prefixed
// name (e.g. "/mylock"), have a single counter initialized to a value on
// creation, and expose Post / Wait / TryWait primitives. Under the hood
// each supported platform uses its own native syscall family:
//
//   - Linux:  System V IPC (semget / semop / semctl) with the name hashed
//     to a 32-bit integer key. SysV syscalls have stable ABI and
//     work reliably via raw syscalls.
//
//   - Darwin: POSIX named semaphores via syscall.SYS_SEM_OPEN and friends.
//     These exist as real kernel traps on darwin — unlike on
//     Linux where sem_open is a libc composite of open + mmap +
//     futex — and have fixed-argument signatures that go through
//     syscall.Syscall cleanly.
//
// Because POSIX sem_open semantics are the narrower of the two, they
// define the exported API. On Linux the extra SysV capabilities
// (semaphore sets, wait-for-zero, semctl commands) are hidden behind the
// same Go interface.
//
// # Hash collision on Linux
//
// The Linux backend hashes the user-facing name to an int32 semget key
// (FNV-1a). Two different names hashing to the same key would silently
// share the same kernel object — a latent correctness hazard. The 2^32
// key space gives a birthday bound around 77,000 names, so practical risk
// is low, but cooperating applications should use a common prefix (e.g.
// "/myapp.v1.<purpose>") so that they compete only with themselves in
// the key space and can't be aliased by unrelated processes.
//
// # Typical lifecycle
//
//	s, err := semaphore.NewSemaphore("/mylock", 0, 1) // create, initial=1
//	defer s.Close()
//	defer s.Unlink()
//
//	s.Wait()  // acquire — blocks until counter is positive, then decrements
//	// ... critical section ...
//	s.Post()  // release — increments, waking one waiter
package semaphore
