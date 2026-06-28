# Rikami

⚠️ **Under active refactor.** Rikami is currently undergoing a total refactor. Most of the core functionality is complete and working. Anything documented here that is **not yet implemented** is clearly marked with a 🚧 **WIP** tag.

**Rikami** is a Helm chart generation and deployment automation toolkit. Its CLI, `rika`, turns a small declarative manifest (`rikami.yaml`) into a ready-to-deploy Helm chart with minimal boilerplate — handling templating, multi-environment values, and secret sealing along the way.

Generated charts depend on the [rikami-charts](https://github.com/b-zago/rikami-charts) Helm library, which provides the actual Kubernetes resource templates (`lib.*`). Rikami only generates the `values-<env>.yaml`, `Chart.yaml`, and `templates/main.yaml` that drive that library.

## Workflow

The whole point of Rikami is to get from "I wrote an app" to "it's running in the cluster" without hand-writing Kubernetes manifests:

1. **Write your application** — an API, a worker, anything that ships as a container image.
2. **Describe it in `rikami.yaml`** using resources (_shards_, _fragments_, _confs_) from the [rikami-base](https://github.com/b-zago/rikami-base) repo. These are Helm-template-like building blocks tailored for the rikami-charts library.
3. **A runner generates the chart.** A CI job (e.g. GitHub Actions via 🚧 **rikami-action**) runs `rika manifest`, which uses the `ci` package to fetch the referenced templates, render them, seal secrets, and emit a complete Helm chart.
4. **The chart is committed and pushed** to your central Kubernetes repo. 🚧 **WIP** — auto-commit/push is done by the runner; locally `rika manifest` just writes the chart to disk.
5. **GitOps reconciles and deploys.** ArgoCD (or any other GitOps tool) watches the k8s repo and rolls out the change.

## Concepts

Rikami's templates live in a separate **base repo** (`base_owner/base_repo` in config, e.g. `rikami-base`) and are fetched on demand over the GitHub GraphQL API. Every reference is **version-pinned** to a git tag/ref with `@`:

- **Shard** — a top-level resource template, e.g. `WebServer`, `Postgres`, `Redis`. Referenced as `WebServer@v0.0.1`. Fetched from `shards/<name>.yaml`.
- **Fragment** — an add-on embedded inside a shard, written as a **capitalized** nested key, e.g. a `Secret@v0.0.1` block under a shard. Fetched from `fragments/<name>.yaml`.
- **Conf** — chart-level config such as `Chart@v0.0.1`, which renders to `Chart.yaml`. Fetched from `confs/<name>.yaml`.

A minimal [rikami.yaml](rikami.yaml):

```yaml
confs:
  Chart@v0.0.1:
    rikamiVersion: 0.2.1 # version of the rikami-charts library to depend on
    appName: app

envs:
  - prod:
      WebServer@v0.0.1:
        image: app:latest
        runsOn: golang
        port: 80
      Database@v0.0.1:
        url: "(( secValue `.env.database.secret` `url` ))"
        schema: "(( fromFile `schema.sql` | toYAML | hindent 6 ))"
      Redis@v0.0.1:
        noop: ""
```

**Multiple instances of a shard** — append `_<alias>` to reuse the same template under a distinct name, e.g. `Postgres@v0.0.1` and `Postgres_1@v0.0.1`.

**Environment inheritance** — `envs` is an ordered list. The first environment acts as the base; every later environment **inherits any key it doesn't override**. So you can put the full definition under `prod` and let `staging` specify only what differs.

**Templating** — values can embed expressions using `(( ... ))`, which are translated to Go/Helm `{{ ... }}` in the generated chart. A few of these (the secret functions, `fromFile`, etc.) are evaluated at generation time. Note that `"(( ... ))"` strips the surrounding quotes in the output — wrap the whole thing in single quotes `'...'` if you need quoting preserved.

**Cross-referencing shards** — you can pull a value from another shard with `(( .Shards.<shard>.<value> ))`, using the shard's alias when you've defined more than one. The expression also has `.App` and `.Domain` available. For example, to wire a web server to a second Postgres instance's generated secret:

```yaml
WebServer@v0.0.1:
  deployment:
    envSecretRefs:
      - "(( .Shards.Postgres_1.secret.name ))"
Postgres_1@v0.0.1:
  image: postgres:18-alpine
```

The output is written to `<appName>/`:

- `Chart.yaml` — from the Chart conf.
- `values-<env>.yaml` — one per environment (e.g. `values-prod.yaml`, `values-staging.yaml`).
- `templates/main.yaml` — includes the relevant `lib.*` templates from rikami-charts.

## The `rika` CLI

- `rika config` — interactively create the config file.
- `rika config -edit <field> -value <val>` — set a single field. Use `-value -` to read a sensitive value from stdin.
- `rika manifest` — read `./rikami.yaml`, prompt for the app name, and generate the Helm chart. The core command.
- `rika params push -env <env>` — push local `.env*` files to AWS SSM (see [Secrets](#secret-handling)).
- `rika params pull -env <env> -params envs,secrets` — pull params from SSM back into local files.
- `rika seal -ns <namespace> -name <name>` — read a value from stdin and emit a sealed-secret encrypted blob.
- `rika login` — log in to the rikami API; stores tokens next to the config as `creds.json`.
- `rika summon` — 🚧 **WIP** — trigger generation/deploy via the rikami API. Currently a stub.

If the app name isn't given to `manifest`, it falls back to `$REPO_NAME`, then to the remote name parsed from `.git/config`.

## Secret handling

Rikami keeps plaintext secrets out of git through two mechanisms that work together.

### AWS SSM Parameter Store

Per-environment env vars and secrets are stored as encrypted `SecureString` JSON parameters under:

```
/<repo>/<env>/envs       # from .env, .env.* (non-secret) files
/<repo>/<env>/secrets    # from .env*.secret files
```

- `rika params push -env prod` globs local `.env*` files, routes any `*.secret` ones to `secrets` and the rest to `envs`, and uploads them.
- `rika params pull -env prod -params envs,secrets` fetches them and rehydrates the local files.

During `rika manifest`, the generator automatically pulls the params it needs for each environment and keeps SSM in sync (e.g. newly generated random secrets are written back).

AWS credentials are read from the standard AWS SDK config chain.

### Sealed Secrets

So secrets can be safely committed to the k8s repo, Rikami encrypts them with [bitnami sealed-secrets](https://github.com/bitnami-labs/sealed-secrets). The cluster's public cert is fetched from the rikami API's `/cert` endpoint.

Secret template functions, usable in `rikami.yaml`:

- `(( secRand ))` — generate a random 32-byte secret, seal it into the chart, and store the plaintext in SSM. Use inside a secret shard/fragment's `data` block.
- `(( secFile `.env.<name>.secret` ))` — seal every value from the named secret file (pulled from SSM).
- `(( secValue `.env.<name>.secret` `key` ))` — seal a single value from a secret file.

`rika seal` exposes the same sealing for ad-hoc use outside chart generation.

### General template functions

Available in `rikami.yaml` value expressions: `default`, `toYAML`, `quote`, `indent`, `nindent`, `hindent`, and `fromFile <path>` (inline a file's contents).

## Configuration

`rika config` writes a JSON file to your OS config dir (e.g. `~/.config/rika/rika.json` on Linux):

- `URL` — base URL of the rikami API.
- `HMAC` — shared secret used to sign API requests (sensitive).
- `base_gh_token` — GitHub token used to read templates from the base repo.
- `base_owner` — owner of the base/template repo.
- `base_repo` — name of the base/template repo (e.g. `rikami-base`).
- `domain` — base domain used when generating routes/hosts.

## Install / build

Requires Go 1.26+.

```sh
# build the CLI
go build -o rika .

# or via goreleaser (linux/windows, arm64/amd64)
goreleaser release --clean
```

The produced binary is named `rika`.

## Related repositories

- [rikami-charts](https://github.com/b-zago/rikami-charts) — the Helm library that generated charts depend on (`oci://ghcr.io/b-zago/rikami-charts`).
- [rikami-base](https://github.com/b-zago/rikami-base) — the shard / fragment / conf templates consumed by `rika manifest`.
- [rikami-api](https://github.com/b-zago/rikami-api) — serves the sealed-secrets cert, handles login, and 🚧 powers fully automated deploys.
- 🚧 **rikami-action** — GitHub Action wrapper around `rika manifest` for CI. Not yet built.

---

This README tracks an actively moving target and may lag slightly behind the code, but it should cover the bulk of what `rika` can do today.
