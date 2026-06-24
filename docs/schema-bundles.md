# Schema Bundles

[Back to documentation index](index.md)

`møbius` validates manifests offline at runtime.

It does that with:
- rendered CRD schemas from the manifests under review, when available
- embedded schema bundles committed in this repository

Runtime does not fetch schemas.

## Files

Schema bundle inputs and outputs:
- source manifest: [../schemasources.yaml](../schemasources.yaml)
- resolved source lock: [../schemas.lock.yaml](../schemas.lock.yaml)
- embedded bundle: [../internal/validate/schemas](../internal/validate/schemas)

## Which Versions Are Embedded?

[../schemas.lock.yaml](../schemas.lock.yaml) is the source of truth for the schema versions embedded in a build. It records the resolved Kubernetes version and every platform schema source such as CloudNativePG, Kyverno, Cilium, Longhorn, Keycloak, and OpenBao.

Use it to answer questions like:
- which Kubernetes schema version is bundled?
- which CNPG or Kyverno release supplied the CRD schemas?
- whether a source uses an explicit version or a resolved `latest` version?

Runtime validation uses the committed embedded bundle from [../internal/validate/schemas](../internal/validate/schemas). It does not fetch newer schema versions dynamically, so updating schema coverage requires refreshing and committing the bundle.

## Runtime Behavior

Schema resolution order:
1. rendered CRD schema from the current rendered manifests
2. embedded schema bundle from the repo
3. no schema available

Validation remains offline-first:
- no runtime network fetches
- no cluster dependency

## Maintenance Workflow

Refresh schemas with:

```bash
make schema-sync
make schema-verify
```

Typical update flow:

1. add or refresh local schema/CRD inputs, or update `schemasources.yaml`
2. run `make schema-sync`
3. review generated changes under `internal/validate/schemas`
4. review resolved versions in `schemas.lock.yaml`
5. run `make schema-verify`
6. commit the schema update

## Source Model

The repository uses repo-local schema maintenance:
- schemas are imported or refreshed into the git repository
- builds embed the committed files
- runtime only consumes embedded files

That means:
- building on-prem only needs the repo contents and Go dependencies
- runtime stays fully offline
