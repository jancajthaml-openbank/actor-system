# actor-system

No nonsense, easiblity extensible actor system support without need or service discovery and actor tracking.

### Local Messages support

When message is recieved from local environment exposes `ProcessLocalMessage` function
which simplest implementation would be

```
func (system ActorSystemSupport) ProcessLocalMessage(msg interface{}, to string, from Coordinates) {
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
func (system ActorSystemSupport) ProcessLocalMessage(msg interface{}, to string, from Coordinates) {
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

When message is recieved from remote environment exposes `ProcessRemoteMessage` function
which simplest implementation would be

```
func (system ActorSystemSupport) ProcessRemoteMessage(parts []string) {
  if len(parts) != 4 {
    log.Warnf("Invalid message received [%+v remote]", parts)
    return
  }

  region, to, sender, payload := parts[0], parts[1], parts[2], parts[3]
  from := Coordinates{
    Name:   sender,
    Region: region,
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
