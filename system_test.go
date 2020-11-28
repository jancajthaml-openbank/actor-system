package actorsystem

import "testing"

func TestStartStop(t *testing.T) {
	system, _ := New("test", "127.0.0.1")

	ack := make(chan bool)
	go func() {
		system.Start()
		close(ack)
	}()
	system.Stop()
	<-ack
}
