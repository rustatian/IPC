package shm

import "testing"

func TestNewSharedMemorySegment(t *testing.T) {
	seg, err := NewSharedMemorySegment("0x1234", 1024, S_IRUSR|S_IWUSR|S_IRGRP|S_IWGRP, IPC_CREAT)
	if err != nil {
		panic(err)
	}

	seg.Write([]byte("fasdsdfasdf"))
	err = seg.Detach()
	if err != nil {
		panic(err)
	}

	seg2, err := NewSharedMemorySegment("0x1234", 0, 0, SHM_RDONLY)
	if err != nil {
		panic(err)
	}
	err = seg2.Detach()
	if err != nil {
		panic(err)
	}
}
