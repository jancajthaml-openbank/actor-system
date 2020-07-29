package actorsystem

import (
	"context"
	zmq "github.com/pebbe/zmq4"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"runtime"
	"testing"
)

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

func init() {
	log.SetOutput(ioutil.Discard)
	masterContext, masterCancel := context.WithCancel(context.Background())
	go relay(masterContext, masterCancel)
}

func TestStartStop(t *testing.T) {
	masterContext, masterCancel := context.WithCancel(context.Background())
	defer masterCancel()

	system := NewSystem(masterContext, "test", "127.0.0.1")

	go system.Start()
	<-system.IsReady
	system.GreenLight()
	system.Stop()
	system.WaitStop()
}

func TestStopOnContextCancel(t *testing.T) {
	masterContext, masterCancel := context.WithCancel(context.Background())
	defer masterCancel()

	ctx, cancel := context.WithCancel(masterContext)
	system := NewSystem(ctx, "test", "127.0.0.1")

	go system.Start()
	<-system.IsReady
	system.GreenLight()
	cancel()
	system.WaitStop()
}
