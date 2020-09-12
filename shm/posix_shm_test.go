package shm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testData = "hello my dear friend"

func TestNewSharedMemorySegment(t *testing.T) {
	testBuf := make([]byte, 0, 0)
	testBuf = append(testBuf, []byte(testData)...)

	seg1, err := NewSharedMemorySegment(0x1, 1024, S_IRUSR|S_IWUSR|S_IRGRP|S_IWGRP, IPC_CREAT)
	if err != nil {
		t.Fatal(err)
	}

	// write data to the shared memory
	seg1.Write([]byte(testData))
	err = seg1.Detach()
	if err != nil {
		t.Fatal(err)
	}

	seg2, err := NewSharedMemorySegment(0x1, 1024, 0, SHM_RDONLY)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(testData), len(testData))
	err = seg2.Read(len(testData), buf)
	if err != nil {
		t.Fatal(err)
	}

	err = seg2.Detach()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, testBuf, buf)
}
