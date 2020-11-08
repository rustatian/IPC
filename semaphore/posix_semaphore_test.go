// +build linux

package semaphore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSemaphore_Add(t *testing.T) {
	s, err := NewSemaphore(0x12334, 1, 0666, true, IPC_CREAT)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Add(0)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		s2, err2 := NewSemaphore(0x12334, 1, 0666, false, IPC_CREAT)
		if err2 != nil {
			assert.Fail(t, "", err2)
		}
		time.Sleep(time.Second * 5)
		err2 = s2.Done(0)
		if err2 != nil {
			assert.Fail(t, "", err2)
		}
	}()

	time.Sleep(time.Second * 1)
	err = s.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
