package semaphore

import "testing"

func TestSemaphore_GetValue(t *testing.T) {
	s, err := NewSemaphore("31231231", 1, 0666, IPC_CREAT)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
