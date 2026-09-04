# NXYZ Local Recovery

If `localhost:8080` is refusing connections or a host-agent script was not executable, update the repository and run:

```bash
git pull
cd nxyz-cloud
make local-up
```

`make local-up` invokes the launcher through Bash, so it works even on archives/filesystems that discard executable bits. The launcher:

1. verifies/installs free local prerequisites on macOS when Homebrew is available;
2. starts an existing Podman machine, initializing one only when none exists;
3. builds the checked-out NXYZ controller and agent binaries;
4. starts the control plane on `127.0.0.1:8080`;
5. waits until `/healthz` is live;
6. starts a rootless Podman execution agent;
7. waits until the node appears in `/api/v1/nodes`;
8. opens the dashboard on macOS.

Stop local NXYZ services with:

```bash
make local-down
```

Logs are stored under `nxyz-cloud/.nxyz/`.
