package main

import (
	"github.com/ValeryPiashchynski/rgoshm/shm"
)

func main() {
	seg, err := shm.NewSharedMemorySegment("424225342", 10000, shm.IPC_CREAT)
	if err != nil {
		panic(err)
	}

	seg.Write([]byte("fasdsdfasdf"))
	dd := seg.Read()
	_ = dd
}
