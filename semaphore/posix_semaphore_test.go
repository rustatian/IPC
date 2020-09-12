package semaphore

import "testing"

func TestSemaphore_GetValue(t *testing.T) {
	s, err := NewSemaphore(0x1234, 1, 0666, IPC_CREAT)
	if err != nil {
		t.Fatal(err)
	}

	// will wait forever
	err = s.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
