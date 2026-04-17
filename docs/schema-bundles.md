# Schema Bundles

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
