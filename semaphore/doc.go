// Package semaphore provides named counting semaphores for interprocess
// coordination via raw system calls — no cgo.
//
// The API is POSIX-flavored: semaphores are identified by a slash-prefixed
// name (e.g. "/mylock"), have a single counter initialized to a value on
// creation, and expose Post / Wait / TryWait primitives. Under the hood
// each supported platform uses its own native syscall family:
//
//   - Linux:  System V IPC (semget / semop / semctl) with the name hashed
//     down to an integer key. SysV syscalls have stable ABI and
//     work reliably via raw syscalls.
//
//   - Darwin: POSIX named semaphores (sem_open / sem_post / sem_wait / ...)
//     which exist as real syscalls (numbers 268–274). Unlike the
//     SysV semctl trap, these have simple non-variadic signatures
//     and go through syscall.Syscall cleanly.
//
// Because POSIX sem_open semantics are the narrower of the two, they
// define the exported API. On Linux the extra SysV capabilities
// (semaphore sets, wait-for-zero, semctl commands) are hidden behind the
// same Go interface.
//
// Typical lifecycle:
//
//	s, err := semaphore.NewSemaphore("/mylock", 0, 1) // create, initial=1
//	defer s.Close()
//	defer s.Unlink()
//
//	s.Wait()  // acquire — blocks until counter is positive, then decrements
//	// ... critical section ...
//	s.Post()  // release — increments, waking one waiter
package semaphore
