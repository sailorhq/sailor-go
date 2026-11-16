# Sailor Consumer Specification

Status: normative. Version 1.0 of the consumer contract for the Sailor v1 backend.

This document specifies how a Sailor **consumer** (a library embedded in an application) reads configuration and secrets from a Sailor server. It is written so that an implementation in any language can be built from this document alone, and checked against the conformance cases in section 16.

The keywords MUST, MUST NOT, SHOULD, SHOULD NOT and MAY are used as defined in RFC 2119.

Every rule has an identifier of the form `CONS-<AREA>-<n>`. Conformance cases refer to these identifiers.

---

## 0. Design principle: keep it simple

A consumer is a small, boring component. Its job is to keep one or more named values current in memory and to tell operators when it cannot.

- **CONS-DES-1** The public surface MUST consist of: a constructor, `Start`, `Close`, and a typed `Get` for each resource. Nothing else is public.
- **CONS-DES-2** Every configuration option MUST change behaviour that a conformance case asserts. Options that are accepted but ignored are forbidden.
- **CONS-DES-3** Each fetch mode MUST share one state-recording path. A mode MAY differ only where the wire protocol or the storage forces it.
- **CONS-DES-4** Scheduling MUST be a loop with a timer and a cancellation signal. Recursion, self-rescheduling callbacks and unbounded goroutine chains are forbidden.
- **CONS-DES-5** The core MUST have no dependencies beyond the language's standard library, except for a JSON decoder and an HTTP client where the standard library is unsuitable. Optional decoders (YAML, TOML) MUST be pluggable and MUST NOT be linked into the core.
- **CONS-DES-6** Configuration MUST be validated once, at construction. After a successful `Start`, the consumer MUST NOT return errors to the application for transient backend trouble. It MUST keep the last good value and report the trouble through the logger (section 11).
- **CONS-DES-7** A consumer MUST NOT fetch its own keys, write resources, discover its own backend, or interpret resource content beyond decoding it.

## 1. Glossary

| Term | Meaning |
|---|---|
| Server | The Sailor v1 HTTP API. |
| Project | The pair of namespace and app, written `{ns}_{app}` in storage and in tokens. |
| Resource | A named, versioned blob inside a project. Stored as bytes, optionally encrypted. |
| Version | A positive integer assigned by the server to each deployment of a resource. |
| Digest | `sha256:` followed by the lowercase hex SHA-256 of the resource's bytes as served. |
| Fetch mode | One of VOLUME, PULL, DEV. Section 7. |
| Plug | A server-side component that materialises an active version outside Sailor, for example a Kubernetes ConfigMap. Consumers never talk to plugs. |
| Fallback | A static copy of a resource, published by a plug, used when the server cannot be reached. Section 7.4. |
| Consumer report | A POST by the consumer stating which digest it is now serving. Section 9. |

## 2. Configuration model

### 2.1 Options

A consumer is built from these inputs:

| Input | Type | Required | Meaning |
|---|---|---|---|
| `Resources` | list of resource definitions | yes, at least one | Section 4. |
| `Addr` | string | when built from fields | Scheme, host and optional port. Section 3. |
| `Namespace` | string | yes | Project namespace. |
| `App` | string | yes | Project app. |
| `AccessKey` | string | yes | Instance access key. Used for decryption salt. |
| `SecretKey` | string | yes | Instance secret key. Used for decryption input. |
| `Token` | string | no | App token, sent as `x-token`. Needed for reports and for protected projects. |
| `Env` | string | no | Environment name. Used only for DEV cache paths and logging. |
| `URI` | string | no | A `sailor://` URI. Section 3. |
| `UseLocalConfig` | bool | no, default false | Read connection from `~/.sailor/config`. Section 12. |
| `Watch` | bool | no, default true | Whether VOLUME and DEV resources are watched for file changes. |
| `RequestTimeout` | duration | no, default 10 s | Timeout for each HTTP request except consumer reports. |
| `ReportTimeout` | duration | no, default 5 s | Timeout for each consumer-report POST. |

- **CONS-CFG-1** An implementation MUST NOT expose any other option. Fields such as `SocketTimeout`, `Logging` and `FallbackEnabled` were accepted and ignored in earlier versions and MUST NOT be reintroduced.
- **CONS-CFG-2** Timeouts MUST be applied to every outbound request. An implementation MUST NOT use an HTTP client with no timeout.

### 2.2 Source precedence

The connection source is chosen by the first rule that matches, in this order:

1. **CONS-CFG-3** If `Resources` is empty, construction MUST fail with `EMPTY_RESOURCES` (section 14).
2. **CONS-CFG-4** If the environment variable `SAILOR_URI` is set and non-empty, it MUST be parsed as a URI (section 3). It REPLACES all connection fields, including any passed in code. Other connection inputs are ignored.
3. **CONS-CFG-5** Otherwise, if `UseLocalConfig` is true, `Namespace` and `App` MUST both be non-empty, and the connection is read from `~/.sailor/config` (section 12). `Namespace` and `App` come from the caller.
4. **CONS-CFG-6** Otherwise, if `URI` is non-empty, it MUST be parsed as a URI (section 3).
5. **CONS-CFG-7** Otherwise, if `Addr` is non-empty, the connection is the fields as given. `Addr` MUST be normalised (section 3.3).
6. **CONS-CFG-8** Otherwise, construction MUST fail with `NO_CONNECTION`.

Precedence is decided at construction. Later changes to environment variables MUST NOT affect a constructed consumer.

### 2.3 Environment variables read by the consumer

| Variable | Read when | Meaning |
|---|---|---|
| `SAILOR_URI` | construction | Connection URI (CONS-CFG-4). |
| `SAILOR_TOKEN` | only when the connection comes from a URI (CONS-CFG-4, CONS-CFG-6) | App token. |
| `SAILOR_FALLBACK_BASE_URL` | when a fallback is needed | Base URL of the fallback location (section 7.4). |
| `POD_NAME`, `POD_IP`, `NODE_NAME` | when building a consumer report | Optional details (section 9.2). |

- **CONS-CFG-9** `SAILOR_TOKEN` MUST NOT be read when the connection comes from fields, `UseLocalConfig`, or `Addr`. In those cases the token is only the `Token` field.
- **CONS-CFG-10** Environment variables not listed above MUST NOT be read.

## 3. Addresses and URIs

### 3.1 URI grammar

```
sailor://<access_key>:<secret_key>@<host>[:<port>]/<namespace>/<app>[?scheme=<http|https>]
```

- **CONS-URI-1** The scheme of the URI MUST be `sailor`. Any other scheme MUST fail with `INVALID_URI_PREFIX`.
- **CONS-URI-2** The userinfo MUST contain both an access key and a secret key, separated by the first `:`. Each part MUST be percent-decoded (RFC 3986). A missing access key MUST fail with `MISSING_ACCESS_KEY`. A missing secret key MUST fail with `MISSING_SECRET_KEY`.
- **CONS-URI-3** The path, with the leading `/` removed, MUST split on `/` into exactly two non-empty components: namespace, then app. Otherwise the URI MUST fail with `MISSING_PATH_COMPONENTS`.
- **CONS-URI-4** The query parameter `scheme`, if present, MUST be `http` or `https`. Any other value MUST fail with `INVALID_SCHEME`.
- **CONS-URI-5** The host (including port) is taken as written, without percent-decoding.

Verified example, from `net/url` in Go:
`sailor://a%40b:c%3Ad@host.example:7766/ns/app?scheme=https` parses to access key `a@b`, secret key `c:d`, host `host.example:7766`, namespace `ns`, app `app`, scheme `https`.

### 3.2 Scheme

- **CONS-URI-6** The address scheme MUST be taken from the `scheme` query parameter when present.
- **CONS-URI-7** When the `scheme` parameter is absent, the scheme MUST default to `http`.
- **CONS-URI-8** When `Addr` is given as fields, a scheme present in `Addr` MUST be respected. When absent, the scheme MUST default to `http`.
- **CONS-URI-9** An explicitly given scheme MUST NEVER be overridden by a default.

### 3.3 Normalisation of `Addr`

- **CONS-URI-10** If `Addr` contains `://`, it MUST be used unchanged.
- **CONS-URI-11** Otherwise `http://` MUST be prepended.
- **CONS-URI-12** An empty `Addr` MUST fail with `NO_CONNECTION` during construction. It MUST NOT be normalised to `http://`.

### 3.4 Transport security

- **CONS-URI-13** When a token is configured and the scheme is `http` and the host is not a loopback address (`localhost`, `127.0.0.0/8`, `::1`), the consumer MUST log `config.insecure_transport` at warn level (section 11).
- **CONS-URI-14** The consumer MUST NOT refuse to run over `http`. The default is `http` by design, and the warning is the safeguard.

Note for implementers: with an `http` default, the `x-token` header and encrypted-resource ciphertext cross the network in clear text unless the operator asks for `https`. Deployments SHOULD set `?scheme=https` or give an explicit `https://` address for any non-loopback host.

## 4. Resource definitions

Each resource is declared with:

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Name` | string | none, required | Resource name on the server. Also the VOLUME file suffix and the DEV cache name. |
| `Lang` | string | `json` | Decoder key (section 4.2). |
| `Encrypted` | bool | false | Whether the server stores the resource encrypted. Declared by the caller. |
| `Path` | string | none | Mount directory for VOLUME. Ignored by other modes. |
| `Mode` | enum | none, required | VOLUME, PULL or DEV. |
| `Once` | bool | false | When true, PULL fetches once and never polls. |
| `PullInterval` | duration | 10 s | Poll period for PULL. |
| `Decode` | function | JSON decode into a typed target | Turns resolved bytes into the stored value. A raw resource stores bytes unchanged. |

- **CONS-RES-1** `Name` MUST be non-empty. Names MUST be unique within one consumer. A duplicate MUST fail construction with `DUPLICATE_RESOURCE`.
- **CONS-RES-2** `PullInterval` MUST default to 10 s when zero or unset. A negative value MUST fail construction with `INVALID_PULL_INTERVAL`. The minimum accepted value is 1 s.
- **CONS-RES-3** `Encrypted` MUST be taken from the declaration, never from the server's `x-resource-encrypted` header. The header is used only for a warning (CONS-CRYPT-9).

### 4.1 Standard resources

| Constructor | Name | Mode | Encrypted | Path |
|---|---|---|---|---|
| ConfigMap default | `config` | VOLUME | false | `/etc/sailor` |
| Secrets default | `secret` | VOLUME | true | `/etc/sailor/secret` |
| Config pull | `config` | PULL | false | none |
| Secrets pull | `secret` | PULL | true | none |
| Config dev | `config` | DEV | false | none |
| Secrets dev | `secret` | DEV | true | none |
| Misc pull | caller-supplied | PULL | false | none, raw bytes |

- **CONS-RES-4** Each standard constructor MUST produce exactly the values in this table. Implementations SHOULD provide them as convenience functions, and MUST NOT provide any others.

### 4.2 Lang and decoders

- **CONS-RES-5** `Lang` MUST be normalised by trimming whitespace, lowercasing, mapping an empty value to `json`, and mapping `yml` to `yaml`.
- **CONS-RES-6** The only decoder built in is `json`.
- **CONS-RES-7** A resource whose normalised `Lang` has no registered decoder MUST fail with `UNSUPPORTED_LANG` at decode time, never silently produce a zero value.
- **CONS-RES-8** Decoders MUST be registered per consumer instance, before `Start`. A registry shared between instances is forbidden (section 10).
- **CONS-RES-9** `Lang` is ignored for encrypted resources. Their decrypted content is always a map of strings (section 6.4).

## 5. Wire protocol

All paths are relative to the server base URL: `{scheme}://{host}`.

### 5.1 Fetching a resource

`GET /api/v1/resource/{ns}/{app}/{name}`

- **CONS-HTTP-1** The request MUST send `x-token` when a token is configured (CONS-HTTP-9).
- **CONS-HTTP-2** The consumer MAY send `If-None-Match` with the last known version. The server does not implement it (see CONS-HTTP-6), so the consumer MUST NOT depend on a 304.
- **CONS-HTTP-3** A 200 response body is the resource content, byte for byte. There is no envelope and no required `Content-Type`. Content may be JSON, YAML, TOML or text.
- **CONS-HTTP-4** A 200 response carries `x-resource-version`, a decimal integer. Its absence MUST be treated as "version unknown", not as an error.
- **CONS-HTTP-5** The response header `x-resource-encrypted` MUST be used only to log a warning when it disagrees with the declaration (CONS-CRYPT-9).
- **CONS-HTTP-6** Current servers send no `ETag` and never return 304. Implementations MUST handle 200 for an unchanged resource and MUST detect the change by digest (section 8), not by status code.

Response codes:

| Status | Body | Meaning | Consumer behaviour |
|---|---|---|---|
| 200 | content | success | decode, store, compare digest |
| 304 | none | not modified (only if a future server sends it) | keep current value |
| 400 | `{"message": ...}` | empty namespace, app or name, or an invalid token | error |
| 401 | `{"message": ...}` | missing token on a protected project, or insufficient role or access | error |
| 403 | `{"message": ...}` | token issuer or access key rejected | error |
| 404 | `{"message": "this resource was never deployed!"}` | no active version, or unknown resource or project | error (fallback applies, section 7) |
| 500 | `{"message": ...}` | server error | error, retry at the next poll |

- **CONS-HTTP-7** Every error body is a JSON object with a `message` string, followed by a newline. A consumer MUST NOT depend on other fields.
- **CONS-HTTP-8** A 400 caused by an expired token is indistinguishable from a 400 for a malformed token. Consumers MUST NOT try to classify it further.

### 5.2 Authentication

- **CONS-HTTP-9** The header name is `x-token`. The value MUST be sent exactly as configured, with no prefix and no `Bearer` scheme.
- **CONS-HTTP-10** A consumer with no token MUST still send requests. The server treats a project that does not yet exist as unauthenticated and answers 404 (see the server's `ClientCallable`), and it rejects a protected project with 401.

### 5.3 Consumer report

`POST /api/v1/resource/{ns}/{app}/{name}/consumer-report`

Request headers: `Content-Type: application/json`, `x-token`.

Request body:

```json
{
  "consumer": "<identity>",
  "version": "<version or empty>",
  "digest": "sha256:<lowercase hex>",
  "details": { "pod_name": "...", "pod_ip": "...", "node_name": "..." }
}
```

- **CONS-HTTP-11** `consumer` MUST be non-empty. The server rejects an empty value with 400.
- **CONS-HTTP-12** `digest` MUST be `sha256:` followed by 64 lowercase hex characters. The server compares digests by exact string equality, so a missing prefix makes the report count as mismatched.
- **CONS-HTTP-13** `version` MAY be empty. It is stored but never compared.
- **CONS-HTTP-14** `details` MUST omit keys whose values are empty. It MUST be omitted entirely when no value is set.
- **CONS-HTTP-15** The request body MUST NOT exceed 4 MiB. The reports defined here are far smaller.
- **CONS-HTTP-16** Success is any 2xx status. The server answers `{"message":"ok"}`.
- **CONS-HTTP-17** A non-2xx response MUST be logged with the status and the first 4096 bytes of the body, as `report.failed` (section 11).

## 6. Encrypted resources

### 6.1 Wire format

The body of an encrypted resource is a JSON object. Each key maps to a record:

```json
{
  "<key>": {
    "encrypted_secret": "<base64>",
    "encrypted_dek": "<base64>"
  }
}
```

- **CONS-CRYPT-1** Both fields are strings containing standard base64 (RFC 4648 section 4): the `+` and `/` alphabet, with `=` padding. URL-safe and unpadded variants MUST be rejected with a decode error.
- **CONS-CRYPT-2** Each record has its own random data-encryption key (DEK), 32 bytes.

### 6.2 Key derivation

- **CONS-CRYPT-3** The key-encryption key (KEK) MUST be derived with HKDF-SHA256 (RFC 5869), extract then expand, with:
  - input key material (IKM): the `SecretKey` as UTF-8 bytes;
  - salt: the `AccessKey` as UTF-8 bytes;
  - info: the empty byte string;
  - output length: 32 bytes.

  No other constants, prefixes or domain-separation strings are used.

**Known-answer vector 1 (KEK).** For `SecretKey = "server-secret-key"` and `AccessKey = "server-access-key"`, the KEK is:

```
9dbd6d9747e4182c853388a93ee76ad5decb1c284e4d9200256e0cbfc63a343d
```

### 6.3 Ciphers and layout

- **CONS-CRYPT-4** Both the DEK wrap and the value encryption MUST use AES-256-GCM with a 12-byte nonce and a 16-byte authentication tag. Additional authenticated data MUST be empty.
- **CONS-CRYPT-5** The decoded layout of each field is `nonce(12) || ciphertext || tag(16)`. The nonce is the first 12 bytes.
- **CONS-CRYPT-6** `encrypted_dek` decodes to exactly 60 bytes (12 + 32 + 16). Any other length MUST fail with `DECRYPT_FAILED`.
- **CONS-CRYPT-7** `encrypted_secret` decodes to 12 + len(plaintext) + 16 bytes. An empty plaintext therefore decodes to 28 bytes.
- **CONS-CRYPT-8** Decoding MUST check the length before slicing. An implementation MUST NOT read the nonce from a buffer shorter than 12 bytes. The reference Go implementation has this defect in `DecryptDEK`, and ports MUST NOT copy it.

**Known-answer vector 2 (decryption).** Given the KEK inputs above, this record MUST decrypt to the map shown:

```json
{
  "db_password": {
    "encrypted_secret": "lKaQNKO3FIX6i37ZNf+afEEEua9pVPOFAGKaLijlaW2pQklTFHtw278=",
    "encrypted_dek": "Hf08s2skJt9HwGX2pptDX6GUCh/K97qQUhIgnFBk7FaqsvmWStMrlmclgPbYHphZ7m4AeT7w0jQ1zgQS"
  },
  "empty": {
    "encrypted_secret": "akkTYxk3ISelMKgWRgElDPkZopM70gE5Bi2hqw==",
    "encrypted_dek": "UV6tqt7KQbHhINb4Kne1N9f9FH+DdFhtcLQQnHjWWDuul8lFr/JUXZFERCayB/X1QPscAcK0Q+rhRa5x"
  }
}
```

Expected plaintext:

```json
{ "db_password": "correct-horse", "empty": "" }
```

This vector was checked by decryption through the reference implementation. Every conformant implementation MUST pass it.

### 6.4 Output

- **CONS-CRYPT-10** The decrypted result MUST be a flat map from key to UTF-8 string. Values MUST be the raw plaintext bytes, interpreted as UTF-8, with no trimming.
- **CONS-CRYPT-11** A decryption error for any key MUST fail the whole resource. A partial map MUST NOT be stored.
- **CONS-CRYPT-12** Decryption MUST NOT run unless the resource is declared `Encrypted` (CONS-RES-3).

### 6.5 Failure classes

| Cause | Error code |
|---|---|
| body is not valid JSON, or a record is malformed | `DECODE_FAILED` |
| base64 decode error | `DECRYPT_FAILED` |
| wrong length after decode | `DECRYPT_FAILED` |
| GCM authentication failure (wrong key, tampered data) | `DECRYPT_FAILED` |

The error MUST name the key when known. It MUST NOT include any part of the ciphertext or plaintext (section 11).

- **CONS-CRYPT-9** When the server's `x-resource-encrypted` header disagrees with the declared `Encrypted`, the consumer MUST log `resource.encryption_mismatch` at warn level and continue using the declaration.

## 7. Fetch modes

Every mode produces a value by calling the shared state path (section 8). Mode-specific behaviour is limited to where the bytes come from and how the consumer waits for changes.

### 7.1 VOLUME

The consumer reads a file that something else mounts and updates.

- **CONS-VOL-1** The file path MUST be `{Path}/_{Name}`.
- **CONS-VOL-2** At start, the file MUST be read. If it is missing or unreadable, the consumer MUST use the fallback (7.4). If the fallback also fails, construction-time `Start` MUST fail with `RESOURCE_NOT_LOADED`.
- **CONS-VOL-3** When `Watch` is true, the consumer MUST watch the directory `{Path}`, not the file, because Kubernetes updates mounts by swapping a symlink.
- **CONS-VOL-4** The consumer MUST react to these filesystem events on the watched directory: create, write, rename, chmod. Other events MAY be ignored.
- **CONS-VOL-5** Events MUST be debounced: after an event, the consumer waits 500 ms with no further events for the same directory before reading. The reference implementation uses a fixed 1 s sleep inside the event loop. The debounce replaces it, and events MUST NOT block one another.
- **CONS-VOL-6** After the debounce, the consumer MUST re-read only the resources whose file path was affected, when the event carries a path. When the event does not carry a path, it MUST re-read all VOLUME resources.
- **CONS-VOL-7** A failed re-read MUST log `resource.decode_failed` or `resource.stale` (section 11) and keep the last good value.

### 7.2 PULL

The consumer fetches from the server on a timer.

- **CONS-PULL-1** The first fetch MUST happen during `Start`. If it fails with a status other than 200, or fails at the transport layer, the consumer MUST use the fallback (7.4). If the fallback also fails, `Start` MUST fail with `RESOURCE_NOT_LOADED`.
- **CONS-PULL-2** When `Once` is true, no further fetches happen after the first successful load.
- **CONS-PULL-3** Otherwise the consumer MUST run a polling loop: wait `PullInterval`, fetch, process, repeat. The loop MUST be implemented as a single goroutine per resource, MUST NOT recurse, and MUST exit when the consumer is closed (section 10).
- **CONS-PULL-4** Each poll MUST send `If-None-Match` with the last known version when one exists (CONS-HTTP-2). A 304 MUST keep the current value and MUST NOT count as a change.
- **CONS-PULL-5** A 200 whose digest equals the stored digest MUST NOT replace the stored value and MUST NOT trigger a report.
- **CONS-PULL-6** After the first successful load, a failing poll (any non-200, transport error, decode error) MUST keep the last good value. It MUST log `resource.stale` once when the failure starts (section 11), and `resource.recovered` once when a poll succeeds again. It MUST NOT log once per failed poll.
- **CONS-PULL-7** A 404 after a successful load MUST be treated like any other failure under CONS-PULL-6. It MUST NOT clear the value.

### 7.3 DEV

For local development. The consumer caches the server's copy on disk and lets a developer hand-edit the cache.

- **CONS-DEV-1** The cache path MUST be `{home}/.sailor/cache/{ns}-{app}-{env}-{name}.json`. `{env}` is `Env`, or the empty string when unset.
- **CONS-DEV-2** The sidecar path MUST be the cache path with `.meta.json` appended. Its content MUST be a JSON object with two fields: `version` (string) and `hash` (lowercase hex SHA-256 of the cache file's bytes at the time it was written).
- **CONS-DEV-3** A cache file is **pristine** when a sidecar exists and `sha256(file bytes)` equals `hash`. Otherwise it is **locally edited**. A missing sidecar MUST count as locally edited.
- **CONS-DEV-4** The decision table at start:

| State | Server answer | Action | Log |
|---|---|---|---|
| no cache file | 200 | write file and sidecar, use content | `resource.loaded` |
| no cache file | error | construction fails with `RESOURCE_NOT_LOADED` | `resource.load_failed` |
| pristine | 304 or same digest | use cache | none |
| pristine | 200, new digest | overwrite file and sidecar, use new content | `resource.changed` |
| pristine | error | use cache | `resource.stale` |
| locally edited | any | use the file as it is; never overwrite | `resource.local_override` |

- **CONS-DEV-5** Files MUST be created with mode 0644 and directories with mode 0755.
- **CONS-DEV-6** `DEV` mode MUST watch the cache file's directory when `Watch` is true, and react to a change in the cache file by reading it as a local edit (CONS-DEV-3), which means it is used as is.
- **CONS-DEV-7** DEV MUST NOT poll. The server is consulted at start only.

### 7.4 Fallback

- **CONS-FB-1** The fallback URL MUST be `{SAILOR_FALLBACK_BASE_URL}/{App}-{Name}.sailor.fall`, with any trailing `/` removed from the base.
- **CONS-FB-2** The request MUST NOT send `x-token`.
- **CONS-FB-3** The response status MUST be 200. Any other status MUST be treated as a fallback failure. The reference implementation stores a 404 body as content, and ports MUST NOT copy it.
- **CONS-FB-4** The fallback content is the raw bytes of the active version. It MUST be processed exactly like a server response: decrypted when `Encrypted`, decoded, then stored (section 8). Its version is unknown.
- **CONS-FB-5** A resource MUST use the fallback only when `FallbackEnabled` would allow it. Because the option is removed (CONS-CFG-1), fallback is always allowed for VOLUME and PULL. DEV MUST NOT use the fallback.
- **CONS-FB-6** `SAILOR_FALLBACK_BASE_URL` unset MUST make the fallback fail immediately with `FALLBACK_FAILED`.
- **CONS-FB-7** When a fallback value is used, the consumer MUST log `resource.fallback_used` (section 11).

## 8. State, versions and digests

Each resource has one state record: `{value, version, digest}`.

- **CONS-STATE-1** The digest is `sha256:` plus the lowercase hex SHA-256 of the bytes as received from the source (server, file or fallback), **before** decryption or decoding. This is the same value the server stores as the resource's digest for the active version.
- **CONS-STATE-2** A change is a digest that differs from the stored digest, or the first load. Version differences alone do not count as a change.
- **CONS-STATE-3** The stored version MUST be the last known version from the source that gave it. A source that gives no version MUST NOT clear a known version. The reference implementation clears it on VOLUME and DEV re-reads, and ports MUST NOT copy that.
- **CONS-STATE-4** The version string is the response `ETag` verbatim when present, otherwise `x-resource-version`. For VOLUME, DEV and fallback reads, the version is unknown.
- **CONS-STATE-5** Only the state path (`store`) may change a resource's value. It MUST run as one step: decrypt if needed, decode, replace the value, update version and digest, then decide on a change. A failure at any step MUST leave the previous state intact.
- **CONS-STATE-6** A change MUST trigger exactly one consumer report for that resource (section 9), and a non-change MUST trigger none, except for heartbeats.

## 9. Consumer reporting

### 9.1 When

- **CONS-RPT-1** A report MUST be sent when a resource's digest changes (CONS-STATE-6).
- **CONS-RPT-2** A heartbeat report MUST be sent every 10 minutes per resource, with the current digest and version, regardless of change.
- **CONS-RPT-3** The heartbeat interval MUST NOT exceed one third of the server's 30-minute freshness window. The default of 10 minutes satisfies this.
- **CONS-RPT-4** Reports MUST NOT block resource loading, polling or `Get`. They MUST run in a goroutine owned by the consumer and bounded by `ReportTimeout`.
- **CONS-RPT-5** A failed report MUST NOT be retried immediately. The next change or heartbeat sends the current state.

### 9.2 Identity and details

- **CONS-RPT-6** The `consumer` identity MUST be the operating system's host name. If it cannot be read or is empty, the value MUST be `unknown`.
- **CONS-RPT-7** `details` MUST carry `pod_name` from `POD_NAME`, `pod_ip` from `POD_IP`, and `node_name` from `NODE_NAME`, each only when set and non-empty (CONS-HTTP-14).

### 9.3 No token

- **CONS-RPT-8** When `Token` is empty, no report MUST be sent. The consumer MUST log `report.skipped_no_token` once per resource (section 11). It MUST NOT log this on every heartbeat.

## 10. Concurrency and lifecycle

- **CONS-CONC-1** All state MUST belong to one consumer instance. Package-level or process-level mutable state is forbidden. Two consumers in one process MUST NOT affect one another.
- **CONS-CONC-2** Construction is pure. It MUST NOT start goroutines, open files or make network calls. Those happen in `Start`.
- **CONS-CONC-3** `Start(ctx)` MUST perform the first load of every resource (sections 7.1 and 7.2), start the background goroutines, and return. It MUST fail if any resource cannot be loaded and has no fallback.
- **CONS-CONC-4** `Close()` MUST stop all background goroutines (polling, watching, heartbeats, reports in flight), close the file watcher, and return only after every goroutine has exited. It MUST be idempotent.
- **CONS-CONC-5** Values MUST be read and replaced atomically per resource. `Get` MUST NOT block on I/O.
- **CONS-CONC-6** Each resource's state MUST be protected by a lock or an equivalent mechanism. A plain mutex per resource is sufficient and is the recommended choice.
- **CONS-CONC-7** The implementation MUST pass a race detector with no reports.
- **CONS-CONC-8** A consumer MUST NOT be used after `Close`. `Get` after `Close` MUST return the last value without error. Reloads after `Close` MUST NOT happen.
- **CONS-CONC-9** Errors returned by `Get` MUST be only `RESOURCE_NOT_LOADED` or `TYPE_MISMATCH`.

### 10.1 Typed access

- **CONS-CONC-10** `Get` MUST return the stored value converted to the requested type. A conversion failure MUST return `TYPE_MISMATCH` and MUST NOT return a zero value without an error.
- **CONS-CONC-11** For a raw resource, `Get` returns the bytes.

## 11. Logging and errors

### 11.1 Logging rules

The consumer runs unattended inside other services. Its logs are often the only record of what it did.

- **CONS-LOG-1** The logger MUST be injectable. The interface MUST offer `debug`, `info`, `warn` and `error` levels, each taking a message and key/value fields. The default implementation writes one structured line per event to standard error.
- **CONS-LOG-2** The consumer MUST NOT write to any global logger and MUST NOT call `exit` or `panic` for any reason covered by this specification.
- **CONS-LOG-3** Every event MUST have a stable event name. The event name is the first field, named `event`. Names in section 11.3 MUST NOT change between versions.
- **CONS-LOG-4** Every event concerning a resource MUST carry `resource`, and events concerning a project MUST carry `namespace` and `app`.
- **CONS-LOG-5** State transitions MUST be logged, not repetitions. A condition that persists MUST be logged once when it starts and once when it clears.
- **CONS-LOG-6** Levels:
  - `error`: the consumer is serving wrong, missing or stale data, and a human should look.
  - `warn`: degraded but still serving a valid value.
  - `info`: a lifecycle event or a change of state.
  - `debug`: per-request detail, off by default.
- **CONS-LOG-7** No event at `info` or above may be emitted per request.
- **CONS-LOG-8** Response bodies MUST NOT be logged, except the first 4096 bytes of a failed consumer report (CONS-HTTP-17).

### 11.2 Redaction

- **CONS-LOG-9** The following MUST NEVER appear in any log at any level: `Token`, `SecretKey`, `AccessKey`, the userinfo of any `sailor://` URI, decrypted values, ciphertext, the contents of any file read by VOLUME or DEV, the contents of `~/.sailor/config`.
- **CONS-LOG-10** Any URI in a log MUST have its userinfo removed.
- **CONS-LOG-11** Digests in logs MUST be truncated to the first 12 hex characters after `sha256:`.
- **CONS-LOG-12** Errors from the crypto path MUST name the key, never a value.

### 11.3 Event catalogue

| Event | Level | Emitted when | Required fields |
|---|---|---|---|
| `config.resolved` | info | construction finished | `source` (env URI, local config, URI option, fields), `addr` (with scheme), `namespace`, `app`, `token` (bool) |
| `config.insecure_transport` | warn | token set, scheme `http`, non-loopback host | `host` |
| `resource.loaded` | info | first successful load | `resource`, `mode`, `version`, `digest` (truncated) |
| `resource.changed` | info | digest differs after a load | `resource`, `mode`, `old_version`, `new_version`, `old_digest`, `new_digest` (truncated) |
| `resource.load_failed` | error | initial load failed and no fallback value exists | `resource`, `mode`, `cause` |
| `resource.fallback_used` | warn | value came from fallback | `resource`, `reason`, `url` (no credentials) |
| `resource.stale` | warn | refresh has started failing after a success | `resource`, `mode`, `cause`, `version`, `seconds_since_success` |
| `resource.recovered` | info | refresh works again after `resource.stale` | `resource`, `version` |
| `resource.decode_failed` | error | decode or decrypt failed on a reload; last value kept | `resource`, `key` (when known), `cause` |
| `resource.local_override` | info | DEV cache is locally edited, upstream not applied | `resource`, `path` |
| `resource.encryption_mismatch` | warn | server header disagrees with declaration | `resource`, `declared`, `server` |
| `report.skipped_no_token` | warn | once per resource | `resource` |
| `report.failed` | warn | report POST failed or non-2xx | `resource`, `status` (0 for transport), `message` (first 4096 bytes, truncated) |
| `report.acknowledged` | debug | report accepted | `resource`, `status` |
| `watcher.failed` | error | the file watcher could not start or reported an error | `path`, `cause` |
| `consumer.started` | info | `Start` returned successfully | `resources` (count) |
| `consumer.stopped` | info | `Close` finished | `resources`, `uptime_seconds` |

- **CONS-LOG-13** The four events that matter most, because they are the only signal for failures users have hit in practice, MUST always be emitted in the conditions above: `report.skipped_no_token`, `resource.stale`, `resource.fallback_used`, and `config.resolved`.
- **CONS-LOG-14** `report.skipped_no_token` is the signal for a consumer that silently is not verifying its deployment. It MUST be emitted even when reporting was never attempted.

## 12. Local developer configuration

`~/.sailor/config` is a JSON object with these fields. Other fields MUST be ignored.

| Field | Type | Use |
|---|---|---|
| `env` | string | name of the active environment |
| `manifest.envs` | list of `{name, host}` | environments; the host of the active one is the address |
| `tokens` | object, environment name to token | the token of the active environment is the app token |
| `user` | string | not used by the consumer |

- **CONS-LOCAL-1** The file MUST be read at construction only when `UseLocalConfig` is true (CONS-CFG-5).
- **CONS-LOCAL-2** The active environment's `host` MUST be normalised per section 3.3. Its scheme, if any, is respected.
- **CONS-LOCAL-3** A missing file MUST fail with `LOCAL_CONFIG_NOT_FOUND`. Invalid JSON MUST fail with `LOCAL_CONFIG_INVALID`. An active environment with no matching manifest entry MUST fail with `LOCAL_CONFIG_ENV_NOT_FOUND`.
- **CONS-LOCAL-4** The token in the file is used only when the caller's `Token` is empty.

## 13. Testability

- **CONS-TEST-1** The following MUST be injectable: HTTP client (or transport), clock, filesystem, environment reader, home directory, host name, and logger.
- **CONS-TEST-2** Tests MUST NOT sleep. Timed behaviour (polling, debounce, heartbeat) MUST be driven by an injected clock or a timer the test can advance.
- **CONS-TEST-3** Tests MUST NOT depend on the real home directory, real environment variables, or the real host name. They MUST inject these.
- **CONS-TEST-4** Every conformance case in section 16 MUST run against an in-process fake server that implements the wire contract in section 5, and MUST NOT require a running Sailor installation.
- **CONS-TEST-5** The suite MUST include a race-detector run.

## 14. Error codes

Errors are identified by stable codes. Messages are free text; codes are part of this specification.

| Code | Returned from | Meaning |
|---|---|---|
| `EMPTY_RESOURCES` | construction | no resources declared |
| `NO_CONNECTION` | construction | no connection source matched |
| `INVALID_URI_PREFIX` | construction | URI scheme is not `sailor` |
| `MISSING_ACCESS_KEY` | construction | URI has no access key |
| `MISSING_SECRET_KEY` | construction | URI has no secret key |
| `MISSING_PATH_COMPONENTS` | construction | URI path is not exactly namespace and app |
| `INVALID_SCHEME` | construction | `scheme` parameter is not `http` or `https` |
| `DUPLICATE_RESOURCE` | construction | two resources share a name |
| `INVALID_PULL_INTERVAL` | construction | negative `PullInterval` |
| `UNSUPPORTED_LANG` | decode | no decoder for the normalised lang |
| `LOCAL_CONFIG_NOT_FOUND` | construction | `~/.sailor/config` missing |
| `LOCAL_CONFIG_INVALID` | construction | `~/.sailor/config` is not valid JSON |
| `LOCAL_CONFIG_ENV_NOT_FOUND` | construction | active environment not in manifest |
| `RESOURCE_NOT_LOADED` | `Start`, `Get` | no value, and no fallback |
| `FALLBACK_FAILED` | fallback | fallback unavailable or non-200 |
| `TYPE_MISMATCH` | `Get` | stored value has a different type |
| `DECODE_FAILED` | decode | content is not valid for its lang |
| `DECRYPT_FAILED` | decrypt | key derivation, base64, length or GCM failure |
| `HTTP_STATUS` | fetch | server answered a non-200 status |

- **CONS-ERR-1** Construction errors MUST be returned from the constructor. `Start` errors MUST be returned from `Start`.
- **CONS-ERR-2** After `Start` succeeds, no error MUST be returned to the application from background work. Such errors MUST be logged (section 11) and the last good value kept.

## 15. Differences from the current reference SDK

This section lists defects in the current `sailor-go` implementation that this specification corrects. It is informative, not normative: the rules above are what an implementation must do.

| Current behaviour | Specified behaviour | Rule |
|---|---|---|
| `SocketTimeout`, `Logging`, `FallbackEnabled` accepted and ignored | removed | CONS-CFG-1 |
| HTTP client with no timeout | timeouts on every request | CONS-CFG-2 |
| `PullInterval` zero means a tight loop | defaults to 10 s | CONS-RES-2 |
| `fetchFallback` stores a 404 body as content | non-200 is a failure | CONS-FB-3 |
| `keepPullingResource` recurses forever | single loop with cancellation | CONS-PULL-3 |
| `watcherFileNameResourceMap` is a package global | per-instance state | CONS-CONC-1 |
| watcher sleeps a fixed 1 s inside the event loop | debounce | CONS-VOL-5 |
| VOLUME and DEV re-reads clear the version | versions are kept | CONS-STATE-3 |
| `fsnotify.NewWatcher` error ignored | logged as `watcher.failed` | section 11.3 |
| PULL failures after success are silent | `resource.stale` and `resource.recovered` | CONS-PULL-6 |
| decryption of a short DEK panics | length check, error | CONS-CRYPT-8 |
| `consumerIdentity` falls back silently | `unknown` with logging | CONS-RPT-6 |
| no `Close`, goroutines never stop | `Close` joins every goroutine | CONS-CONC-4 |
| `os.Getenv`, `os.Hostname`, `os.UserHomeDir` called directly | injected | CONS-TEST-1 |

Server-side gaps observed during the audit, which the consumer design does not depend on. They are listed for follow-up and are not assumed by any rule above:

- The server sends no `ETag` and never returns 304 (CONS-HTTP-6).
- An expired token returns 400, not 401 (CONS-HTTP-8).
- The server does not enforce revocation of app tokens on the request path. This was not fully verified.
- The v1 server has no admission webhook that injects `SAILOR_URI`. The v0 webhook emits a URI with no scheme, which defaults to `http` under CONS-URI-7.

## 16. Conformance cases

Each case names the rules it exercises and the expected result. A conformant implementation MUST pass every case. Cases are numbered `C-<n>`.

### 16.1 Configuration

| Case | Input | Expected | Rules |
|---|---|---|---|
| C-1 | no resources | `EMPTY_RESOURCES` | CONS-CFG-3 |
| C-2 | `SAILOR_URI` set, `Addr` also set in code | connection comes from the URI | CONS-CFG-4 |
| C-3 | `UseLocalConfig` true, no namespace | error | CONS-CFG-5 |
| C-4 | nothing set | `NO_CONNECTION` | CONS-CFG-8 |
| C-5 | `SAILOR_TOKEN` set, connection from fields | token is the field value, env ignored | CONS-CFG-9 |

### 16.2 URI

| Case | Input | Expected | Rules |
|---|---|---|---|
| C-6 | `sailor://a%40b:c%3Ad@host:7766/ns/app?scheme=https` | access `a@b`, secret `c:d`, host `host:7766`, scheme `https`, ns `ns`, app `app` | CONS-URI-2, CONS-URI-5 |
| C-7 | `sailor://a:b@host/ns/app` | scheme `http` | CONS-URI-7 |
| C-8 | `sailor://a:b@host/ns/app?scheme=ftp` | `INVALID_SCHEME` | CONS-URI-4 |
| C-9 | `http://a:b@host/ns/app` | `INVALID_URI_PREFIX` | CONS-URI-1 |
| C-10 | `sailor://:b@host/ns/app` | `MISSING_ACCESS_KEY` | CONS-URI-2 |
| C-11 | `sailor://a@host/ns/app` | `MISSING_SECRET_KEY` | CONS-URI-2 |
| C-12 | `sailor://a:b@host/ns` | `MISSING_PATH_COMPONENTS` | CONS-URI-3 |
| C-13 | `sailor://a:b@host/ns/app/extra` | `MISSING_PATH_COMPONENTS` | CONS-URI-3 |
| C-14 | `Addr = "host:7766"` | normalised to `http://host:7766` | CONS-URI-11 |
| C-15 | `Addr = "https://host"` | unchanged | CONS-URI-10 |
| C-16 | `Addr = ""` | `NO_CONNECTION` | CONS-URI-12 |
| C-17 | token set, `http://host` (non-loopback) | `config.insecure_transport` at warn | CONS-URI-13 |
| C-18 | token set, `http://localhost` | no insecure warning | CONS-URI-13 |

### 16.3 Wire and auth

| Case | Setup | Expected | Rules |
|---|---|---|---|
| C-19 | server returns 200 with raw JSON | bytes stored unchanged | CONS-HTTP-3 |
| C-20 | 200 without `x-resource-version` | version unknown, no error | CONS-HTTP-4 |
| C-21 | 404 `never deployed` at start, no fallback | `RESOURCE_NOT_LOADED` | CONS-PULL-1 |
| C-22 | token configured, server returns 401 | error, `x-token` header sent | CONS-HTTP-9, CONS-HTTP-10 |
| C-23 | server returns a 304 only when `If-None-Match` matches | value kept, no change | CONS-PULL-4 |
| C-24 | server ignores `If-None-Match` and returns 200 with the same digest | no change, no report | CONS-PULL-5 |

### 16.4 Encryption

| Case | Input | Expected | Rules |
|---|---|---|---|
| C-25 | KAT vector 2 with KEK inputs | plaintext map as in 6.3 | CONS-CRYPT-3, CONS-CRYPT-4, CONS-CRYPT-10 |
| C-26 | KEK for the same inputs | `9dbd6d97…a343d` | CONS-CRYPT-3 |
| C-27 | wrong secret key | `DECRYPT_FAILED`, no partial map | CONS-CRYPT-11 |
| C-28 | one ciphertext byte flipped | `DECRYPT_FAILED` | CONS-CRYPT-4 |
| C-29 | `encrypted_dek` decodes to 11 bytes | `DECRYPT_FAILED`, no panic | CONS-CRYPT-6, CONS-CRYPT-8 |
| C-30 | URL-safe base64 in a field | `DECRYPT_FAILED` | CONS-CRYPT-1 |
| C-31 | empty plaintext, record decodes to 28 bytes | value is `""` | CONS-CRYPT-7 |
| C-32 | server header `x-resource-encrypted: false`, declared encrypted | warning logged, declaration used | CONS-CRYPT-9 |
| C-33 | non-ASCII plaintext `héllo wörld 🔐` | round-trips as UTF-8 | CONS-CRYPT-10 |

### 16.5 Fetch modes and fallback

| Case | Setup | Expected | Rules |
|---|---|---|---|
| C-34 | VOLUME file present | value read from `{Path}/_{Name}` | CONS-VOL-1 |
| C-35 | VOLUME file missing, fallback 200 | fallback used, `resource.fallback_used` | CONS-VOL-2, CONS-FB-7 |
| C-36 | VOLUME file changes via symlink swap | value updated after debounce, one report | CONS-VOL-3, CONS-VOL-5, CONS-STATE-6 |
| C-37 | VOLUME re-read with no version | previous version kept | CONS-STATE-3 |
| C-38 | PULL, poll fails for 30 intervals after success | one `resource.stale`, value unchanged | CONS-PULL-6 |
| C-39 | PULL, poll recovers | one `resource.recovered` | CONS-PULL-6 |
| C-40 | PULL 404 after success | value kept, treated as failure | CONS-PULL-7 |
| C-41 | PULL `Once` true | exactly one request | CONS-PULL-2 |
| C-42 | fallback returns 404 with body | `FALLBACK_FAILED`, body not stored | CONS-FB-3 |
| C-43 | `SAILOR_FALLBACK_BASE_URL` unset | `FALLBACK_FAILED` immediately | CONS-FB-6 |
| C-44 | DEV, cold cache, server 200 | cache and sidecar written, modes 0644 and 0755 | CONS-DEV-4, CONS-DEV-5 |
| C-45 | DEV, pristine, server 200 new digest | cache and sidecar overwritten | CONS-DEV-4 |
| C-46 | DEV, pristine, server unreachable | cache used, `resource.stale` | CONS-DEV-4 |
| C-47 | DEV, cache edited by hand | file kept, `resource.local_override`, not overwritten | CONS-DEV-3, CONS-DEV-4 |
| C-48 | DEV, sidecar missing | treated as locally edited | CONS-DEV-3 |
| C-49 | DEV, no polling after start | no requests after start | CONS-DEV-7 |

### 16.6 State and reporting

| Case | Setup | Expected | Rules |
|---|---|---|---|
| C-50 | first load | `resource.loaded`, one report | CONS-STATE-2, CONS-RPT-1 |
| C-51 | digest changes | `resource.changed`, one report | CONS-STATE-6 |
| C-52 | digest unchanged | no report | CONS-STATE-6 |
| C-53 | heartbeat interval elapses | one report with unchanged digest | CONS-RPT-2 |
| C-54 | report digest | `sha256:` plus 64 lowercase hex | CONS-HTTP-12, CONS-STATE-1 |
| C-55 | report to server that returns 500 | `report.failed` with status and truncated body | CONS-HTTP-17 |
| C-56 | no token | no report sent, `report.skipped_no_token` logged once | CONS-RPT-8 |
| C-57 | `POD_NAME` unset | `details` has no `pod_name` key | CONS-RPT-7, CONS-HTTP-14 |
| C-58 | host name unreadable | consumer is `unknown` | CONS-RPT-6 |
| C-59 | report blocks in network | `Get` still returns promptly | CONS-RPT-4 |

### 16.7 Lifecycle and logging

| Case | Setup | Expected | Rules |
|---|---|---|---|
| C-60 | two consumers in one process | independent state | CONS-CONC-1 |
| C-61 | `Close` during polling | all goroutines exit, `Close` returns | CONS-CONC-4 |
| C-62 | `Close` twice | no error, no panic | CONS-CONC-4 |
| C-63 | `Get` after `Close` | last value, no error | CONS-CONC-8 |
| C-64 | race detector over the whole suite | no reports | CONS-CONC-7 |
| C-65 | secrets in every path | none appears in any captured log line | CONS-LOG-9 |
| C-66 | URI with userinfo logged | userinfo removed | CONS-LOG-10 |
| C-67 | 30 failing polls | at most one `resource.stale` line | CONS-LOG-5 |
| C-68 | every event in 11.3 emitted by the matching condition | event name, level and required fields as listed | CONS-LOG-3, CONS-LOG-4 |
| C-69 | `Get` with wrong type | `TYPE_MISMATCH` | CONS-CONC-10 |

## 17. Open items

- **Remote scheme encoding.** The `scheme` query parameter is the chosen encoding. Its interaction with the v0 webhook, which emits no scheme, is covered by CONS-URI-7.
- **Server ETag and 304.** The consumer is designed to work without them. Adding them later would let PULL skip decoding entirely and is a backend change outside this specification.
- **Revocation.** Whether the server rejects revoked app tokens on the request path has not been verified. The consumer design does not depend on it.
- **Heartbeat tuning.** The 10-minute interval is fixed by CONS-RPT-2. A future change must keep CONS-RPT-3 true.

## 18. Implementation notes and test mapping

This section records decisions made while implementing sections 1 to 17. Where it adds to or narrows an earlier rule, the rule text here governs.

### 18.1 Clarifications

- **CONS-CFG-9 (token on the URI option path).** When the connection comes from `Connection.URI`, a non-empty `Connection.Token` wins. Otherwise `SAILOR_TOKEN` is used. When the connection comes from `SAILOR_URI`, the token is `SAILOR_TOKEN` only.
- **CONS-CFG-5 (local config).** Namespace and App are required, and a missing value fails with `MISSING_NAMESPACE_APP`. The caller's `Token`, when set, overrides the token in the file (CONS-LOCAL-4).
- **Fields connection.** A connection built from `Addr` also requires `Namespace` and `App`, and fails with `MISSING_NAMESPACE_APP` when either is empty.
- **`Start(ctx)`.** The context bounds the initial loads only. Background work runs until `Close`, and is not cancelled by the caller's context.
- **Error codes added.** `EMPTY_RESOURCE_NAME`, `INVALID_URI` (a URI that does not parse at all), `MISSING_NAMESPACE_APP`, `ALREADY_STARTED`, `INVALID_FETCH_MODE` (no mode, or VOLUME without a path). These extend section 14.
- **`HTTP_STATUS` removed.** A non-200 answer never surfaces as its own code. It appears in the `cause` of `resource.load_failed` or `resource.stale`, and a failed initial load returns `RESOURCE_NOT_LOADED`.
- **Public options.** `UseLocalConfig`, `RequestTimeout`, `ReportTimeout`, `Logger`, `HTTPClient` and `Decoders` are the only options (CONS-CFG-1). `HTTPClient` and `Logger` are public because callers need them. The clock, filesystem, environment, home directory, host name and watcher are internal seams.
- **Decoders.** Decoders are per consumer instance. `Decoders` is keyed by lang, and keys are normalised as in CONS-RES-5.
- **Encrypted lang.** An encrypted resource is always decoded as JSON, whatever its declared lang (CONS-RES-9).
- **Initial report.** The first load counts as a change and produces one report. A consumer with no token logs `report.skipped_no_token` during `Start`, once for each resource, whether or not it ever attempts a report (CONS-LOG-14).
- **DEV, server down at start.** A pristine cache that cannot be revalidated is used, and `resource.stale` is logged (CONS-DEV-4). A cold cache with an unreachable server fails `Start` with `RESOURCE_NOT_LOADED`, and DEV never uses a fallback.
- **Watcher failures.** A watcher that cannot start or add a directory is logged as `watcher.failed` and the consumer keeps serving its current values (CONS-ERR-2). The `path` field is empty when the watcher itself cannot be created.
- **Decode failures on reload.** `resource.decode_failed` is logged once per distinct bad payload. `resource.stale` is logged once per transition. The last good value is kept.

### 18.2 Test mapping

Every conformance case has a test. Some cases share one test function, and two are not test functions.

| Cases | Test |
|---|---|
| C-1 | `TestC1_EmptyResourcesFails` |
| C-2 | `TestC2_EnvURIReplacesConnection` |
| C-3 | `TestC3_LocalConfigNeedsNamespaceAndApp` |
| C-4 | `TestC4_NoConnectionFails` |
| C-5 | `TestC5_SailorTokenIgnoredForFieldConnection` |
| C-6 | `TestC6_URIUserinfoIsPercentDecoded` |
| C-7 | `TestC7_URIDefaultsToHTTP` |
| C-8 to C-13 | `TestC8ToC13_URIErrors` (table) |
| C-14 to C-16 | `TestC14ToC16_AddrNormalisation` |
| C-17, C-18 | `TestC17And18_InsecureTransportWarning` (table) |
| C-19 to C-24 | `TestC19_...` through `TestC24_...` |
| C-25 | `TestC25_KnownAnswerDecryption` |
| C-26 | `TestC26_KEKKnownAnswer` |
| C-27 to C-33 | `TestC27_...` through `TestC33_...`; C-32 is `TestC32_EncryptedHeaderMismatchIsLoggedAndDeclarationWins` |
| C-34 to C-37 | `TestC34_...` through `TestC37_...` |
| C-38 | `TestC38_OutageLogsOnceAndKeepsValue`. This is also C-67, which describes the same scenario. |
| C-39 to C-41 | `TestC39_...` through `TestC41_...` |
| C-42, C-43 | `TestC42_...`, `TestC43_...` |
| C-44 to C-49 | `TestC44_...` through `TestC49_...` |
| C-50 to C-59 | `TestC50_...` through `TestC59_...` |
| C-60 to C-63 | `TestC60_...` through `TestC63_...` |
| C-64 | Not a test function. It is the race-detector run: `go test -race ./...`. |
| C-65, C-66 | `TestC65_SecretsNeverLogged`, `TestC66_URIUserinfoRedactedInLogs` |
| C-67 | See C-38. |
| C-68 | `TestC68_CriticalEventsCarryFields`. It checks a sample of the events in 11.3, not all of them. |
| C-69 | `TestC69_WrongTypeIsAnError` |

Tests that are not conformance cases cover failure paths and the production seams: `TestValidationErrors`, `TestPollDecodeFailureLoggedOnce`, `TestVolumeWatchUnmatchedEventRereadsAll`, `TestWatcherFailuresAreLogged`, the real filesystem, clock and fsnotify adapters, and the public constructors.
