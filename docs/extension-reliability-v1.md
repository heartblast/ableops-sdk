# Optional Extension v1 reliability contracts

These additions preserve protocol 1 and the existing Extension interfaces. Old
manifests may omit every field below. Hosts that predate these additions must be
excluded with the package's Host version constraint before relying on them.

## Configuration references and choices

`config.schema[].type: secretRef` identifies an opaque reference **string**, never
credential bytes. The Host must authorize that reference against this Extension's
existing secret.use bindings before saving, displaying or sending it. This type
does not grant secret.ref, resolve values, or introduce a raw secret API. Package
defaults and `secret: true` are refused for this type. Existing non-secret string
reference fields retain their Host-specific compatibility behavior.

A `select` field may declare `options: [{value: safe, label: Safe}]`. Hosts enforce
membership; defaults must be members. Omitted options retain the legacy free
string contract. Options are optional on the wire and contain no credentials.

## HTTP methods

`routes[].methods: [GET, POST]` restricts a declared backend path. Omission retains
legacy runtime method discovery. Methods are uppercase standard methods; TRACE,
CONNECT, duplicates and unknown methods are refused. GET permits HEAD, matching
Go net/http semantics. The process SDK rejects a runtime route whose method
contradicts its matching declaration. This restriction never substitutes for
Host permission, capability or request-lease checks.

## Persistent data directory

The trusted Host may supply `ABLEOPS_EXT_DATA_DIR`. The process SDK exposes its
absolute canonical path as `Environment.DataDir` and `HostContext.DataDir`; an
empty value means unavailable. The SDK neither chooses nor creates this path,
and never includes it in its environment logging summary. A Host must select a
confined directory outside the installed package and preserve user data across
package replacement and ordinary uninstall. The contract promises no replication,
shared-filesystem safety, data migration reversal or cross-node session transfer.
Do not accept this directory from browser requests or Extension configuration.

Gateway read-only replicas omit the writable data directory because the shared
installation belongs to the Admin. Replica Extensions requiring persistent
metadata must continue to use their explicitly configured shared backend.

## Configuration reload

Config hot reload remains deferred. No validate/apply ACK or reload support is
advertised by this revision. Existing Hosts must drain requests and restart when
settings change, restoring the previous configuration on failure. A future
optional ACK contract needs process-generation binding, exact config digest,
bounded validate/apply phases and a documented rollback guarantee before use.
