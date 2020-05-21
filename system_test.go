package actorsystem

import (
	"context"
	//"fmt"
	"io/ioutil"
	//"runtime"
	//"sync"
	log "github.com/sirupsen/logrus"
	"testing"
	"time"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	system := NewSystem(ctx, "test", "127.0.0.1")

	t.Log("by daemon support ( Start -> Stop )")
	{
		go system.Start()
		<-system.IsReady
		system.GreenLight()
		system.Stop()
		system.WaitStop()
	}
}

func TestStopOnContextCancel(t *testing.T) {
	t.Log("stop with cancelation of context")
	{
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)

		system := NewSystem(ctx, "test", "127.0.0.1")

		go system.Start()
		<-system.IsReady
		system.GreenLight()
		cancel()
		system.WaitStop()
	}
}
