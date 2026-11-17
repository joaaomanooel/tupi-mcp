Task: exclude nested environment files from Docker build contexts.
Change: `.dockerignore` now uses `**/.env`, `**/.env.*`, and `!**/.env.example`.
Assertions: Python pattern/order check — PASS.
Diff check: `git diff --check` — PASS.
Docker check: `docker version --format '{{.Client.Version}} {{.Server.Version}}'` — client 29.9.1; daemon unavailable.
Review: root and nested examples match the intended exclusion/exception order; no build-context run was possible.
