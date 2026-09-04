# NXYZ Agentic Workflow

NXYZ uses a logical agent-fanout model to accelerate cloud operations without spawning unsafe or wasteful millions of operating-system processes.

## Execution lanes

1. **Control lane** — scheduling, health, node state, workload lifecycle.
2. **Build lane** — Git clone, project detection, OCI build, registry push.
3. **Data lane** — storage buckets, PostgreSQL, backups.
4. **Security lane** — encrypted secrets, registry allowlisting, bearer-token mesh identity.
5. **Observability lane** — metrics, logs, workload/container status.
6. **AI lane** — llama.cpp API and AI/agent workloads.
7. **Network lane** — service records, named upstream routes, LAN mesh joins.

Each lane can create many jobs, but concurrency is bounded by the real CPU/RAM advertised by NXYZ nodes. This is the practical implementation of the high-scale agentic workflow: massive logical task decomposition with finite, scheduler-enforced physical execution.
