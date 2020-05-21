package actorsystem

import (
	"context"
	"io/ioutil"
	"runtime"
	zmq "github.com/pebbe/zmq4"
	log "github.com/sirupsen/logrus"
	"testing"
	"time"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func relay(ctx context.Context, cancel context.CancelFunc) {
	runtime.LockOSThread()
	defer func() {
		recover()
		cancel()
		runtime.UnlockOSThread()
	}()

	var (
		chunk string
		pull  *zmq.Socket
		pub   *zmq.Socket
		err   error
	)

	zCtx, err := zmq.NewContext()
	if err != nil {
		panic(err.Error())
	}
	defer zCtx.Term()

	pull, err = zCtx.NewSocket(zmq.PULL)
	if err != nil {
		panic(err.Error())
	}
	defer pull.Close()

	pub, err = zCtx.NewSocket(zmq.PUB)
	if err != nil {
		panic(err.Error())
	}
	defer pub.Close()

	err = pull.Bind("tcp://127.0.0.1:5562")
	if err != nil {
		panic(err.Error())
	}

	err = pub.Bind("tcp://127.0.0.1:5561")
	if err != nil {
		panic(err.Error())
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			chunk, err = pull.Recv(0)
			if err != nil {
				return
			}
			_, err = pub.Send(chunk, 0)
			if err != nil {
				return
			}
		}
	}
}

func TestStartStop(t *testing.T) {
	masterContext, masterCancel := context.WithCancel(context.Background())
	defer masterCancel()
	go relay(masterContext, masterCancel)

	system := NewSystem(masterContext, "test", "127.0.0.1")

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
	masterContext, masterCancel := context.WithCancel(context.Background())
	defer masterCancel()
	go relay(masterContext, masterCancel)

	t.Log("stop with cancelation of context")
	{
		ctx, cancel := context.WithTimeout(masterContext, time.Second*5)
		system := NewSystem(ctx, "test", "127.0.0.1")

		go system.Start()
		<-system.IsReady
		system.GreenLight()
		cancel()
		system.WaitStop()
	}
}
