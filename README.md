# actor-system

No nonsense, easiblity extensible actor system support without need or service discovery and actor tracking.

### Features

- Multiple actor systems in single service
- No zipkin, no kafka, no heavy weight framework
- State aware actors with actor enveloping
- csp on actor level, parallel on application level
- stash and blacklog support on actor instance
- Spray actor or do something else on message receive JIT with `ProcessLocalMessage` and `ProcessRemoteMessage`

### Simplest Example

```go
import (
  "context"
  "fmt"

  system "github.com/jancajthaml-openbank/actor-system"
)

type ActorSystem struct {
  system.Support
}

func NewActorSystem() ActorSystem {
  ctx := context.Background()
  region := "MyRegion"
  lakeEndpoint := "127.0.0.1"

  return ActorSystem{
    System: system.NewSupport(ctx, region, lakeEndpoint),
  }
}

func (s ActorSystem) ProcessMessage(msg string, to system.Coordinates, from system.Coordinates) {
  fmt.Printf("%+v -> %+v says %s\n", from, to, msg)
}

func main() {
  instance := NewActorSystem()

  instance.ActorSystemSupport.RegisterOnMessage(as.ProcessMessage)

  instance.Start()
}
```

### Messaging

Uses [lake](https://github.com/jancajthaml-openbank/lake) relay for remote messages relay.

When message is recieved from remote environment function registered by `RegisterOnRemoteMessage` is called.
The simplest implementation of such function would be

```go
func (s ActorSystem) ProcessMessage(msg string, to system.Coordinates, from system.Coordinates) {

  var message interface{}

  switch msg {

  case "X":
    message = new(XMessage)

  default:
    message = new(DefaultMessage)
  }

  s.ProcessLocalMessage(message, to, from)
}
```

if you want to spray actors on-demand on messages

```go
func EchoActor(s ActorSystemSupport) func(State, Context) {
  return func(state State, context Context) {
    defer s.UnregisterActor(context.Receiver.Name)
    log.Debug("Echo actor in %+v received %+v", state, context)
  }
}
```

```go
func (s ActorSystemSupport) ProcessLocalMessage(msg interface{}, to system.Coordinates, from system.Coordinates) {
  ref, err := s.ActorOf(to)
  if err != nil {
    ref = actor.NewEnvelope(to)
    err = s.RegisterActor(envelope, EchoActor(s))
    if err != nil {
      log.Warnf("Unable to register Actor [%s local]", to)
      return
    }
  }
  ref.Tell(msg, from)
}
```

You don't need to have actor instance to send message to remote region, you can use `system.SendMessage` function instead.

### Lifecycle control

ActorSystemSupport has methods `Start` for proper system start an `Stop` for gracefull shutdown.

## License

Licensed under Apache 2.0 see [LICENSE.md](https://github.com/jancajthaml-openbank/lake-client/blob/master/LICENSE.md) for details

## Contributing

1. Fork it
2. Create your feature branch (`git checkout -b feature/my-new-feature`)
3. Commit your changes (`git commit -am 'Add some feature'`)
4. Push to the branch (`git push origin feature/my-new-feature`)
5. Create new Pull Request

## Responsible Disclosure

I take the security of my systems seriously, and I value input from the security community. The disclosure of security vulnerabilities helps me ensure the security and integrity of my systems. If you believe you've found a security vulnerability in one of my systems or services please [tell me via email](mailto:jan.cajthaml@gmail.com).

## Author

Jan Cajthaml (a.k.a johnny)
