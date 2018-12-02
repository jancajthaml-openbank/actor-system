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

```
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
  name := "ActorSystem"

  return ActorSystem{
    ActorSystemSupport: system.NewActorSystemSupport(ctx, name, "localhost"),
  }
}

func (s ActorSystem) ProcessLocalMessage(msg interface{}, receiver s.Coordinates, sender s.Coordinates) {
  fmt.Printf("Inherited Actor System recieved local message %+v\n", msg)
}

func (s ActorSystem) ProcessRemoteMessage(parts []string) {
  fmt.Printf("Inherited Actor System recieved remote message %+v\n", parts)
}

func main() {
  as := NewActorSystem()

  as.ActorSystemSupport.RegisterOnLocalMessage(as.ProcessLocalMessage)
  as.ActorSystemSupport.RegisterOnRemoteMessage(as.ProcessRemoteMessage)

  as.Start()
}
```

---

### Local Messages support

When message is recieved from local environment function registered by `RegisterOnLocalMessage` is called.
The simplest implementation of such function would be

```
func (system ActorSystemSupport) ProcessLocalMessage(msg interface{}, to s.Coordinates, from Coordinates) {
  ref, err := system.ActorOf(to)
  if err != nil {
    log.Warnf("Actor not found [%s local]", to)
    return
  }

  ref.Tell(msg, from)
}

```

if you want to spray actors on-demand on local messages

```
func EchoActor(system ActorSystemSupport) func(State, Context) {
  return func(state State, context Context) {
    defer system.UnregisterActor(context.Receiver.Name)
    log.Debug("Echo actor in %+v received %+v", state, context)
  }
}
```

```

actorSystem := ActorSystem{
  ActorSystemSupport: system.NewActorSystemSupport(ctx, "Vault/"+cfg.Tenant, cfg.LakeHostname),
  storage:            cfg.RootStorage,
  tenant:             cfg.Tenant,
  metrics:            metrics,
}

return actorSystem

func (system ActorSystemSupport) ProcessLocalMessage(msg interface{}, to s.Coordinates, from Coordinates) {
  ref, err := system.ActorOf(to)
  if err != nil {
    ref = actor.NewEnvelope(to)
    err = system.RegisterActor(envelope, EchoActor(system))
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

```
func (system ActorSystemSupport) ProcessRemoteMessage(parts []string) {
  if len(parts) != 4 {
    log.Warnf("Invalid message received [%+v remote]", parts)
    return
  }

  region, reciever, sender, payload := parts[0], parts[1], parts[2], parts[3]
  from := Coordinates{
    Name:   sender,
    Region: region,
  }

  to := Coordinates{
    Name:   reciever,
    Region: system.Name,
  }

  system.ProcessLocalMessage(payload, to, from)
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
