# Unix shared memory and semaphores

<p align="center">
	<a href="https://github.com/rustatian/IPC/actions"><img src="https://github.com/rustatian/IPC/workflows/CI/badge.svg" alt=""></a>
</p>

A Go library for Unix interprocess communication — SysV/POSIX shared
memory segments and cross-platform named counting semaphores. Implemented
entirely via raw system calls; **no cgo required**.

## Platform support

| Package      | Platforms        | Backing syscalls |
|--------------|------------------|------------------|
| `shm`        | Linux, darwin    | SysV shm syscalls via `golang.org/x/sys/unix` |
| `semaphore`  | Linux, darwin    | SysV on Linux, POSIX `sem_*` on darwin — unified Go API |

The `semaphore` package presents a single name-based API on every
platform, dispatching to the native syscall family that works best there:

- **Linux** — SysV IPC (`semget`/`semop`/`semctl`). The user-facing name
  is FNV-hashed to an integer `semget` key.
- **Darwin** — POSIX named semaphores (`sem_open`/`sem_post`/`sem_wait`
  and friends). These are real kernel syscalls on darwin (numbers
  268–274), unlike on Linux where `sem_open` is a libc composite of
  `open`+`mmap`+`futex`.

## Semaphores (interprocess)

Process 1 — create a named semaphore with initial value 0 and wait for a
signal:

```go
s, err := semaphore.NewSemaphore("/mylock", 0666, 0)
if err != nil {
    panic(err)
}
defer s.Close()
defer s.Unlink()

if err := s.Wait(); err != nil {
    panic(err)
}
```

Process 2 — attach to the same semaphore and release the waiter:

```go
s, err := semaphore.OpenSemaphore("/mylock")
if err != nil {
    panic(err)
}
defer s.Close()

if err := s.Post(); err != nil {
    panic(err)
}
```

Other operations:

- `OpenOrCreateSemaphore(name, perm, initial)` — idempotent open/create.
- `TryWait()` — non-blocking decrement; returns `syscall.EAGAIN` when the
  counter is zero.

## Shared Memory (interprocess)

Create a shared memory segment with a key, size, and creation flags:

```go
seg1, err := shm.NewSharedMemorySegment(0x1, 1024,
    shm.SIrusr|shm.SIwusr|shm.SIrgrp|shm.SIwgrp, shm.IpcCreat)
if err != nil {
    t.Fatal(err)
}
```

Write data and detach:

```go
if err := seg1.Write([]byte("hello")); err != nil {
    t.Fatal(err)
}
if err := seg1.Detach(); err != nil {
    t.Fatal(err)
}
```

From another process, attach with the same key:

```go
seg2, err := shm.NewSharedMemorySegment(0x1, 1024, 0, shm.Rdonly)
if err != nil {
    t.Fatal(err)
}
```

Read data and detach:

```go
buf := make([]byte, 1024)
n, err := seg2.Read(buf)
if err != nil {
    t.Fatal(err)
}
if err := seg2.Detach(); err != nil {
    t.Fatal(err)
}
_ = buf[:n]
```

When the last process is done with the segment, call `Remove()` to
destroy it (equivalent to `shmctl(IPC_RMID)`).

## Requirements

- Go 1.26+
- No cgo
- Unix platform (Linux or darwin)
