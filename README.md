# Unix shared memory and semaphores

<p align="center">
	<a href="https://github.com/rustatian/IPC/actions"><img src="https://github.com/rustatian/IPC/workflows/CI/badge.svg" alt=""></a>
</p>

A Go library for Unix interprocess communication — System V shared memory,
Linux-only POSIX shared memory, and cross-platform named counting
semaphores. Implemented entirely via raw system calls; **no cgo required**.

## Platform support

| Package                   | Platforms        | Implementation |
|---------------------------|------------------|----------------|
| `shm` — SysV segments     | Linux, darwin    | `SysvShmGet` / `SysvShmAttach` / `SysvShmDetach` / `SysvShmCtl` via `golang.org/x/sys/unix` |
| `shm` — POSIX `/dev/shm`  | Linux only       | `open` + `ftruncate` + `mmap` + `unlink` raw syscalls; equivalent to glibc's `shm_open(3)` |
| `semaphore`               | Linux, darwin    | SysV (`SEMGET`/`SEMOP`/`SEMCTL`) on Linux; POSIX `sem_*` syscalls on darwin — unified Go API |

The `semaphore` package presents a single name-based API on every
platform, dispatching to the native syscall family that works best there:

- **Linux** — SysV IPC. The user-facing name is FNV-hashed to an int32
  `semget` key.
- **Darwin** — POSIX named semaphores via `syscall.SYS_SEM_OPEN` and
  friends. These are real kernel syscalls on darwin, unlike on Linux
  where `sem_open` is a libc composite.

## Semaphores (interprocess)

Process 1 — create a named semaphore with initial value 0 and wait for a
signal:

```go
s, err := semaphore.NewSemaphore("/myapp.v1.mylock", 0666, 0)
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
s, err := semaphore.OpenSemaphore("/myapp.v1.mylock")
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
  The `initial` value only takes effect on actual creation.
- `TryWait()` — non-blocking decrement; returns `syscall.EAGAIN` when the
  counter is zero.

### Hash-collision note (Linux)

On Linux, the semaphore name is hashed to a 32-bit `semget` key. Two
different names hashing to the same key would silently share the same
kernel object. Use application-unique prefixes (e.g. `/myapp.v1.<purpose>`)
to stay within your own key space; see `semaphore/doc.go` for details.

## Shared Memory (interprocess)

### System V segments (portable)

Create a segment with a key, size, and creation flags:

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

### POSIX segments (Linux only)

For name-addressed segments backed by `/dev/shm`:

```go
seg, err := shm.NewSharedMemoryPosix("/myapp.v1.buffer", 4096, 0600)
if err != nil {
    panic(err)
}
defer seg.Detach()
defer seg.Remove() // unlinks /dev/shm/myapp.v1.buffer
```

The name must start with `/`, contain no other slashes or `..`, and be
non-empty.

## Requirements

- Go 1.26+
- No cgo
- Unix platform: Linux or darwin
