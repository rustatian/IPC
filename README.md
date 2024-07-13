# Linux and Windows implementation of shared memory and semaphores

<p align="center">
	<a href="https://github.com/rustatian/IPC/actions"><img src="https://github.com/rustatian/IPC/workflows/CI/badge.svg" alt=""></a>
	<a href="https://lgtm.com/projects/g/rustatian/IPC/alerts/"><img src="https://img.shields.io/lgtm/alerts/g/rustatian/IPC.svg?logo=lgtm&logoWidth=18"></a>
</p>

# How to use
## Semaphores (interprocess):

Process1: Initialize a semaphore, and Add to semaphores set (semaphore 0 in example) value 1. And start doing some interprocess work (write to the shared memory for example).
```go
s, err := NewSemaphore(0x12334, 1, 0666, true, IPC_CREAT)
if err != nil {
	panic(err)
}
err = s.Add(0)
if err != nil {
	panic(err)
}
  ```
  
  After work will be done, just unlock semaphore:
  
  ```go
  err = s.Done(0) // 0 here is the 1-st semaphore in the set identified by semaphore ID.
	if err != nil {
		panic(err)
	}
  ```
  
  Process2: Attach to the same semaphore. And `Wait` until Process1 released semaphore.
  ```go
	s, err := NewSemaphore(0x12334, 1, 0666, false, IPC_CREAT)
	if err != nil {
		panic(err)
	}
	err = s.Wait()
	if err != nil {
		panic(err)
	}
  ```

## Shared Memory (interprocess):

Initialize shared memory segment with a key, required size and flags:
```go
seg1, err := NewSharedMemorySegment(0x1, 1024, S_IRUSR|S_IWUSR|S_IRGRP|S_IWGRP, IPC_CREAT)
if err != nil {
	t.Fatal(err)
}
```  

Write the specified amount of data and detach from the segment:
```go
// write data to the shared memory
// testData is less or equal to 1024 specified in prev declaration 
seg1.Write([]byte(testData))
err = seg1.Detach()
if err != nil {
	t.Fatal(err)
}
```

From the another process, initialize shared memory segment with the same key, size, but with ReadOnly flag:

```go
seg2, err := NewSharedMemorySegment(0x1, 1024, 0, SHM_RDONLY)
if err != nil {
	t.Fatal(err)
}
```

Read specified amount of data and detach from the segment:

```go
buf := make([]byte, len(testData), len(testData))
err = seg2.Read(buf)
if err != nil {
	t.Fatal(err)
}
err = seg2.Detach()
if err != nil {
	t.Fatal(err)
}

```
