# Remote Docker development

Instance directory: `/home/opsadmin/loretide-dev`; Compose project: `loretide-dev`.

Use the dedicated opsadmin SSH key and verified host key file. No root or password SSH login. Docker operations use `sudo -n` inside that authenticated session.

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

## Agent authentication boundary

This environment does not mount the operator's Codex/Claude credentials, SSH key, Docker socket or home directory. The API stores/distributes authorized tasks; a future local daemon must execute the chosen local client on the operator's computer and return authorized progress/artifacts. Client authentication tokens must not be copied to this server.

Upstream runtime credential-link/copy code has not yet been replaced or security-verified. Do not pair a real daemon, configure an AI provider, or execute real model tasks until that work passes its dedicated checks. Absence of a daemon in this Compose file is not proof that all upstream execution paths have been hardened.

See the parent repository's dated deployment record for actual validation and remaining failures. A running container alone does not prove authentication, browser behavior, migrations or recovery passed.
