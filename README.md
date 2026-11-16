# sailor-go

The Go consumer for Sailor. It keeps named configuration and secret resources current in memory, from a mounted file, from the Sailor server, or from a local development cache.

The behaviour is specified in [SAILOR_CONSUMER.md](SAILOR_CONSUMER.md). That document is the contract for this library and for any implementation in another language.

## Install

```bash
go get github.com/sailorhq/sailor-go
```

## Quick start

```go
type AppConfig struct {
    Port     int    `json:"port"`
    LogLevel string `json:"log_level"`
}

consumer, err := sailor.NewConsumer(opts.InitOption{
    Connection: &opts.ConnectionOption{
        Addr:      "http://localhost:7766",
        Namespace: "myteam",
        App:       "hello-world",
        AccessKey: accessKey,
        SecretKey: secretKey,
        Token:     appToken,
    },
    Resources: []opts.ResourceOption{
        sailor.ConfigPullDefault[AppConfig](),
    },
})
if err != nil {
    log.Fatal(err)
}
if err := consumer.Start(context.Background()); err != nil {
    log.Fatal(err)
}
defer consumer.Close()

cfg, err := sailor.Get[AppConfig](consumer, "config")
```

`Start` loads every resource once and then keeps them current in the background. `Close` stops that work and waits for it. `Get` never blocks on I/O.

## Connecting

The connection comes from the first source that matches, in this order:

1. `SAILOR_URI` environment variable. It replaces the whole connection.
2. `UseLocalConfig: true`, which reads `~/.sailor/config`. `Namespace` and `App` are still required from the caller.
3. `Connection.URI`.
4. The fields of `Connection` (`Addr`, `Namespace`, `App`, `AccessKey`, `SecretKey`, `Token`).

A URI has this form:

```
sailor://<access_key>:<secret_key>@<host[:port]>/<namespace>/<app>[?scheme=http|https]
```

The scheme defaults to `http`. Pass `?scheme=https` for an HTTPS server. A bare `Addr` such as `host:7766` also defaults to `http`. An explicit scheme is never overridden.

The token comes from `Connection.Token` when set. Otherwise it comes from the `SAILOR_TOKEN` environment variable, which is read only on the URI paths.

Transport warning: when a token is set and the scheme is `http` for a non-loopback host, the consumer logs `config.insecure_transport`. Use `https` for any server reached over a network.

## Resources

| Constructor | Name | Mode | Encrypted | Source |
|---|---|---|---|---|
| `ConfigMapDefault[T]()` | `config` | VOLUME | no | `/etc/sailor/_config` |
| `SecretsDefault[T]()` | `secret` | VOLUME | yes | `/etc/sailor/secret/_secret` |
| `ConfigPullDefault[T]()` | `config` | PULL | no | server, every 10 s |
| `SecretsPullDefault[T]()` | `secret` | PULL | yes | server, every 10 s |
| `ConfigDevDefault[T]()` | `config` | DEV | no | `~/.sailor/cache` |
| `SecretsDevDefault[T]()` | `secret` | DEV | yes | `~/.sailor/cache` |
| `MiscPullDefault(name)` | name | PULL | no | server, raw bytes |

For your own resources, build them with `sailor.Resource[T](def, fetch)` or `sailor.Raw(def, fetch)`. Read a raw resource with `sailor.Get[[]byte]`.

- **VOLUME** reads a file that something else mounts and updates. A change to the file is picked up after a short debounce.
- **PULL** polls the server. A poll that fails keeps the last good value and logs once when the outage starts and once when it recovers.
- **DEV** caches the server copy under `~/.sailor/cache` for local work. A cache file that you have edited by hand is never overwritten.

A VOLUME or PULL resource that cannot be loaded falls back to `SAILOR_FALLBACK_BASE_URL/{app}-{name}.sailor.fall`, when that variable is set.

## Encrypted resources

Encrypted resources are decrypted inside the consumer. The key-encryption key is derived with HKDF-SHA256 from the secret key, using the access key as salt. Each value is AES-256-GCM encrypted under its own data key. The exact wire format and two known-answer vectors are in SAILOR_CONSUMER.md, section 6.

## Consumer reports

With a token configured, the consumer reports the digest it is serving when a resource changes, and again every 10 minutes. The Sailor console uses these reports for its deployment verification screen. Without a token the consumer logs `report.skipped_no_token` once per resource and sends nothing.

## Logging

Events are logged through an injectable `opts.Logger`. The default writes structured lines to standard error. Event names, levels and required fields are listed in SAILOR_CONSUMER.md, section 11.3. Secrets, tokens and decrypted values are never logged.

The four events to watch in production:

- `report.skipped_no_token`: the consumer is not reporting its state.
- `resource.stale`: the consumer is serving an old value.
- `resource.fallback_used`: the consumer is not talking to Sailor.
- `config.resolved`: the address, scheme and token source that were chosen.

## Options

| Option | Default | Purpose |
|---|---|---|
| `Watch` | `true` | Watch VOLUME and DEV files for changes. |
| `UseLocalConfig` | `false` | Read the connection from `~/.sailor/config`. |
| `RequestTimeout` | 10 s | Timeout for each fetch. |
| `ReportTimeout` | 5 s | Timeout for each consumer report. |
| `Logger` | structured, stderr | Receives every event. |
| `HTTPClient` | `&http.Client{}` | Performs every request. |
| `Decoders` | JSON only | Decoders for other languages, such as YAML or TOML, keyed by lang. |

Decoders belong to the consumer instance, so two consumers can use different decoders.

## Errors

Errors carry a stable code. Match them with `errors.Is` against the exported sentinels, for example `sailor.ErrResourceNotLoaded`. The full list is in SAILOR_CONSUMER.md, section 14.

## Environment variables

| Variable | Used for |
|---|---|
| `SAILOR_URI` | Connection URI (highest precedence). |
| `SAILOR_TOKEN` | App token, on the URI paths only. |
| `SAILOR_FALLBACK_BASE_URL` | Base URL of fallback files. |
| `POD_NAME`, `POD_IP`, `NODE_NAME` | Optional details sent with consumer reports. |

## Development

```bash
go test -race ./...
```

The tests drive timers with a fake clock and use an in-memory filesystem and a fake server. They do not sleep and do not need a running Sailor installation. The mapping from conformance cases to tests is in SAILOR_CONSUMER.md, section 18.
