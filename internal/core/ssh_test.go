package core

import "testing"

func TestRemoteClientCloseCallsCloseAll(t *testing.T) {
	called := 0
	client := &remoteClient{
		closeAll: func() error {
			called++
			return nil
		},
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("closeAll calls = %d, want 1", called)
	}
}

func TestRemoteClientCloseAllowsNilClient(t *testing.T) {
	client := &remoteClient{}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}
