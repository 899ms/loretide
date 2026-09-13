# Remote Docker development

Current host: `78.47.42.189`; instance directory: `/root/loretide-dev`; Compose project: `loretide-dev`.

For this new host, the operator explicitly chose root password authentication. Enter the password interactively; never store it in scripts, command arguments, or version control. The old host `78.47.43.8` still uses opsadmin and its dedicated SSH key, but its Loretide development deployment was removed on 2026-09-13 and the original services restored.

From the instance directory:

```sh
sudo -n docker compose --env-file .env.loretide-dev -f compose.loretide-dev.yaml config --quiet
sudo -n docker compose --env-file .env.loretide-dev -f compose.loretide-dev.yaml up -d
sudo -n docker compose --env-file .env.loretide-dev -f compose.loretide-dev.yaml ps
sudo -n docker compose --env-file .env.loretide-dev -f compose.loretide-dev.yaml stop
```

Stop preserves the database volume. Never use `down -v` for routine stopping. Do not operate the existing `multica` Compose project.

Web binds host loopback 13000, API loopback 18000; database has no host port. Browser access requires an SSH tunnel forwarding these ports. Configuration secrets were generated on the server in an ignored mode-600 file; do not print resolved Compose configuration or commit secrets.

Source is mounted from the dedicated directory. Web uses the pinned pnpm version and hot reload; API rebuilds on container restart. No desktop or daemon service is defined. Dependencies and caches are for this instance. Resource caps limit individual services but do not guarantee that simultaneous builds have no effect on other applications.

## Attachment persistence

`LOCAL_UPLOAD_DIR=/workspace/server/data/uploads` resolves through the existing source bind mount to `/root/loretide-dev/server/data/uploads`. This preserves the upstream default location and restored files. It is separate from the original Multica instance and ignored by Git. Container recreation retains files; deleting the host directory does not. Back up this directory alongside the database.

This is storage for explicitly uploaded application attachments. It does not scan or upload the operator's local media folders. The planned local asset references remain a separate, authorized daemon capability.

## Agent authentication boundary

This environment does not mount the operator's Codex/Claude credentials, SSH key, Docker socket or home directory. The API stores/distributes authorized tasks; a future local daemon must execute the chosen local client on the operator's computer and return authorized progress/artifacts. Client authentication tokens must not be copied to this server.

Upstream runtime credential-link/copy code has not yet been replaced or security-verified. Do not pair a real daemon, configure an AI provider, or execute real model tasks until that work passes its dedicated checks. Absence of a daemon in this Compose file is not proof that all upstream execution paths have been hardened.

See the parent repository's dated deployment record for actual validation and remaining failures. A running container alone does not prove authentication, browser behavior, migrations or recovery passed.
