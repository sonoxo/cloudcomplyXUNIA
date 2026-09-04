# NXYZ Cloud — Free Tool Plane

NXYZ Tool Plane turns the existing local control plane and rootless Podman executor into a usable private-cloud toolkit without AWS, Azure, GCP, Docker Desktop, or paid APIs.

## Install / refresh the CLI

```bash
git pull --ff-only
make install-cli
hash -r
```

Then use `nxyz` from any directory.

## Tool map

| Flow | Command | Runtime |
|---|---|---|
| Cloud start | `nxyz` | NXYZ Go control plane + agent |
| Storage | `nxyz storage ...` | local durable buckets under `.nxyz/tools/storage` |
| Deploy | `nxyz deploy image ...` / `nxyz deploy git ...` | scheduler + rootless Podman |
| Database | `nxyz db ...` | PostgreSQL 16 rootless containers |
| Monitor | `nxyz monitor` | control-plane summary + Podman stats |
| Logs | `nxyz logs [WORKLOAD_ID]` | platform logs / OCI logs |
| Backups | `nxyz backup ...` | tar archives + `pg_dumpall` |
| Secrets | `nxyz secrets ...` | OpenSSL AES-256-CBC + PBKDF2 |
| Registry | `nxyz registry ...` | local OCI Distribution registry on `127.0.0.1:5000` |
| Git | `nxyz git ...` | local bare repositories |
| AI | `nxyz ai ...` | llama.cpp / `llama-server` |
| Agents | `nxyz agents ...` | ordinary NXYZ scheduled workloads |
| Discovery | `nxyz dns ...` | NXYZ service records |
| Terminal | `nxyz terminal WORKLOAD_ID` | rootless `podman exec` |
| Catalog | `nxyz catalog ...` | vetted starter images |
| Proxy records | `nxyz proxy ...` | named upstream registry |
| Mesh | `nxyz mesh ...` | shared cluster token + LAN listener flow |

## Fast examples

```bash
# Object storage
nxyz storage create media
nxyz storage put media ./cover.png
nxyz storage list media

# Public OCI deployment
nxyz deploy image web nginx:alpine 250 128

# Git -> build -> local registry -> schedule -> execute
nxyz deploy git myapp https://github.com/example/myapp.git 500 512

# PostgreSQL
nxyz db create xunia
nxyz db list
nxyz db url xunia

# Encrypted secret
nxyz secrets set API_KEY 'example-value'
nxyz secrets get API_KEY

# Private Git
nxyz git create internal-project
nxyz git path internal-project

# Local AI API (GGUF model supplied by the operator)
nxyz ai install
nxyz ai serve /path/to/model.gguf 8090

# Backups
nxyz backup create before-upgrade
nxyz backup list
nxyz backup restore before-upgrade

# Service discovery
nxyz dns add api 127.0.0.1:8080
nxyz dns resolve api

# App catalog
nxyz catalog list
nxyz catalog install nginx demo

# Multi-node preparation
nxyz mesh token
nxyz mesh status
```

## Agentic build model

The “7 million agent” directive is implemented as a logical fan-out pattern rather than literally launching millions of host processes. Each capability is an independent workstream sharing a common control plane, runtime, state root, security boundary, and CLI. This keeps NXYZ fast and composable without exhausting the machine that supplies the cloud's physical CPU and RAM.

## Boundaries

NXYZ remains local-first. Storage capacity, RAM, CPU, and database disk are supplied by connected machines. `dns` currently provides NXYZ service-discovery records rather than rewriting the host operating system resolver, and `proxy` stores named upstream routes rather than claiming automatic public Internet ingress. Those boundaries keep the zero-cost path deterministic and avoid requiring privileged host modifications.
