# SystemV
System V implementation of  shared memory and semaphores to syncronize.

[WIP]

#### How to use
##### Semaphores (interprocess):

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
	s, err := NewSemaphore(0x12334, 1, 0666, true, IPC_CREAT)
	if err != nil {
		panic(err)
	}
	err = s.Wait()
	if err != nil {
		panic(err)
	}
  ```
