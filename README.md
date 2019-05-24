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
  system.ActorSystemSupport
}

func NewActorSystem() ActorSystem {
  ctx := context.Background()
  name := "MyRegion"
  lake := "127.0.0.1"

  return ActorSystem{
    ActorSystemSupport: system.NewActorSystemSupport(ctx, name, lake),
  }
}

func (s ActorSystem) ProcessLocalMessage(msg interface{}, receiver system.Coordinates, sender system.Coordinates) {
  fmt.Printf("Inherited Actor System recieved local message %+v\n", msg)
}

func (s ActorSystem) ProcessRemoteMessage(parts []string) {
  fmt.Printf("Inherited Actor System recieved remote message %+v\n", parts)
}

func main() {
  instance := NewActorSystem()

  instance.ActorSystemSupport.RegisterOnLocalMessage(as.ProcessLocalMessage)
  instance.ActorSystemSupport.RegisterOnRemoteMessage(as.ProcessRemoteMessage)

  instance.Start()
}
```

---

### Local Messages support

When message is recieved from local environment function registered by `RegisterOnLocalMessage` is called.
The simplest implementation of such function would be

```go
func (s ActorSystemSupport) ProcessLocalMessage(msg interface{}, to s.Coordinates, from Coordinates) {
  ref, err := s.ActorOf(to)
  if err != nil {
    log.Warnf("Actor not found [%s local]", to)
    return
  }

  ref.Tell(msg, from)
}
```

if you want to spray actors on-demand on local messages

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

### Remote messages support

Uses [lake](https://github.com/jancajthaml-openbank/lake) relay for remote messages relay.

When message is recieved from remote environment function registered by `RegisterOnRemoteMessage` is called.
The simplest implementation of such function would be

```go
func (s ActorSystemSupport) ProcessRemoteMessage(msg string) {
  parts := strings.Split(msg, " ")

  if len(parts) < 4 {
    log.Warnf("Invalid message received [%+v remote]", parts)
    return
  }

  recieverRegion, senderRegion, receiverName, senderName := parts[0], parts[1], parts[2], parts[3]

  from := system.Coordinates{
    Name:   senderName,
    Region: senderRegion,
  }

  to := system.Coordinates{
    Name:   receiverName,
    Region: recieverRegion,
  }
  
  var message interface{}

  switch parts[4] {

  case "X": 
    message = new(XMessage)

  default:
    message = new(DefaultMessage)
  }
    
  s.ProcessLocalMessage(message, to, from)
}
```

You don't need to have actor instance to send message to remote region, you can use `system.SendRemote` function instead.

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
