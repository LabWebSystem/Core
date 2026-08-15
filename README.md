# LabWebSystem

LabWebSystem (LWS) is a Docker Compose based LAN web-app platform. Backend, Dashboard and SDK are intentionally mock components in this foundation.

## Development

```sh
mise run lint
mise run test
mise run package
mise run deploy --version 1.2.3
mise run deploy --version 1.2.3 --force
```

`deploy` requires an authenticated `gh` CLI, pushes the `lws-vX.Y.Z` tag, and asks before replacing an existing release unless `--force` is supplied.

## Installation and lifecycle

The GitHub-hosted installer is `scripts/install.sh`. Set `LWS_REPOSITORY=owner/repository` when using a fork:

```sh
curl -fsSL https://raw.githubusercontent.com/owner/repository/main/scripts/install.sh | sudo LWS_REPOSITORY=owner/repository bash
sudo lwsctl start --domain example.internal
sudo lwsctl stop
sudo lwsctl uninstall
sudo lwsctl uninstall --purge --force
```

Package installation only places files. `lwsctl start` starts the Compose project. APT/DNF pre-remove hooks stop it when the package manager is used directly. LWS uses its own Compose project and labels, so unrelated Docker resources are not removed.
