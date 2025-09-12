# 🚀 GLASS — GitLab Awesome Sync System

> ⚡ **In short:** a single binary that pulls *all GitLab projects visible to you* — fast, safe, with two modes: **working-tree** and **mirror**.  
> Perfect as a daily “bootstrap & sync” tool for developers and a lightweight backup for teams.

---

## 🧭 Why

* **Quick start** — with one run you get the full tree of repositories you have access to.
* **Daily synchronization** — pulls new repositories and your colleagues’ changes.
* **Safety by default** — carefully updates working trees without breaking local changes.
* **Simple DevOps tool** — handy flags, clear logs, can run via cron/systemd.

---

## 🔧 Features

* **Modes**:
  * `--mirror` — bare mirror for DevOps (`git clone --mirror`, all branches/tags/refs).
  * `--mirror=false` — default, a regular clone with working tree; updates via `fetch --all --prune`, optionally `checkout` the default branch.
* **Filters**: by namespace (regexp), by activity (`--since`), by size (`--max-size-mb`).
* **Protocols**: HTTPS+PAT (default) or `--ssh` for SSH-URLs.
* **Safe updates**: `--safe-update` (skip if tree is dirty), `--force-reset` (hard reset).
* **Other**: pagination 100/page, retries+backoff, `--prune-local` (remove “orphans”), submodule support.

---

## 📥 Installation

Requires: `git`, `go >= 1.24`, `curl` or `wget`.

```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/xakepp35/glass/main/install.sh)"
````

or:

```bash
sh -c "$(wget -qO- https://raw.githubusercontent.com/xakepp35/glass/main/install.sh)"
```

Binary will be installed to `/usr/local/bin/glass`.

---

## ⚡ Quick Start (FFF)

1. Configure `~/.netrc`:

   ```text
   machine git.my.com
   login oauth2
   password glpat-xxxxxxxxxxxx
   ```

   👉 Important: `chmod 600 ~/.netrc`

2. Add to `~/.bashrc` or `~/.zshrc`:

   ```bash
   export GL_BASE_URL="https://git.my.com"
   export DEST_DIR="$HOME/git.my.com"
   ```

   then reload session:

   ```bash
   source ~/.bashrc   # or source ~/.zshrc
   ```

3. Run:

   ```bash
   glass
   ```

👉 All projects will be pulled into `~/git.my.com`.

---

## 🛠 Usage

| Role               | Command                                                                 | Comment                              |
| ------------------ | ----------------------------------------------------------------------- | ------------------------------------ |
| 👶 New dev         | `glass -mirror=false -checkout-default -j 4`                            | bootstrap all accessible repos       |
| 👨‍💻 Developer    | `glass -mirror=false -safe-update -since=168h -j 8`                     | daily sync of repos active in 7 days |
| 🧑‍🤝‍🧑 Team lead | `glass -mirror=false -checkout-default -archived -j 12`                 | full sync including archived ones    |
| 🔒 SRE/DevOps      | `glass -mirror -prune-local -archived -j 16`                            | nightly mirror backup                |
| 🧪 QA              | `glass -mirror=false -include='^qa-projects/' -recurse-submodules -j 4` | only QA repos with submodules        |
| SSH instead of PAT | `glass -mirror=false -ssh`                                              | git over SSH, API over PAT in netrc  |
| Only namespace     | `glass -mirror=false -include='^team-x/' -exclude='/legacy-' -j 6`      | selective                            |

---

## ⚙️ Flags and environment variables

| Flag                  | ENV equivalent       | Description                                                     | Default           |
| --------------------- | -------------------- | --------------------------------------------------------------- | ----------------- |
| `-base-url`           | `GL_BASE_URL`        | GitLab base URL (`https://gitlab.com`/self-hosted)              | —                 |
| `-token`              | `GL_TOKEN`           | Personal Access Token (PAT) with `read_api` + `read_repository` | —                 |
| `-dest`               | `DEST_DIR`           | Destination root folder                                         | `./gitlab-backup` |
| `-j`                  | `CONCURRENCY`        | Concurrency (workers)                                           | `4`               |
| `-dry-run`            | `DRY_RUN`            | Show planned actions, do not execute                            | `false`           |
| `-membership`         | `MEMBERSHIP`         | Only projects where you are a member                            | `true`            |
| `-min-access`         | `MIN_ACCESS`         | Min. access level (10..50)                                      | `10`              |
| `-archived`           | `INCLUDE_ARCHIVED`   | Include archived projects                                       | `false`           |
| `-http-verbose`       | `HTTP_VERBOSE`       | Log HTTP requests                                               | `false`           |
| `-timeout`            | `TIMEOUT`            | HTTP request timeout                                            | `30s`             |
| `-mirror`             | `MIRROR`             | `git clone --mirror` mode                                       | `false`           |
| `-recurse-submodules` | `RECURSE_SUBMODULES` | Recurse submodules (non-mirror)                                 | `false`           |
| `-checkout-default`   | `CHECKOUT_DEFAULT`   | Checkout default branch after fetch                             | `true`            |
| `-ssh`                | `SSH_MODE`           | Use SSH-URL instead of HTTPS+PAT                                | `false`           |
| `-safe-update`        | `SAFE_UPDATE`        | Skip checkout/pull if tree is dirty                             | `true`            |
| `-force-reset`        | `FORCE_RESET`        | `reset --hard` to `origin/<default>` (dangerous)                | `false`           |
| `-include`            | `INCLUDE`            | Regexp include filter by `path_with_namespace`                  | —                 |
| `-exclude`            | `EXCLUDE`            | Regexp exclude filter by `path_with_namespace`                  | —                 |
| `-since`              | `SINCE`              | Only projects active during period (`72h`, `7d`)                | `0` (all)         |
| `-max-size-mb`        | `MAX_SIZE_MB`        | Skip projects larger than N MB                                  | `0` (off)         |
| `-prune-local`        | `PRUNE_LOCAL`        | Delete local repos not in API                                   | `false`           |
| `-git-timeout`        | `GIT_TIMEOUT`        | Timeout for a single `git` command                              | `10m`             |
| `-use-netrc-api`      | `USE_NETRC_API`      | Read PAT for API from `~/.netrc` if `-token` is empty           | `true`            |
| `-use-netrc-git`      | `USE_NETRC_GIT`      | Do not embed PAT in HTTPS URL, let git use `.netrc`             | `true`            |
| `-netrc`              | `NETRC`              | Path to `.netrc`                                                | `~/.netrc`        |

---

## 🔒 Behavior and safety

* **HTTPS+PAT**: token is injected into git URL only at runtime, never logged.
* **SSH mode**: uses `SSHURLToRepo` and your SSH keys.
* **Safe Update**: if working tree is dirty, GLASS only runs `fetch`.
* **Force Reset**: use only for CI/backups.

---

## ⏰ systemd integration

`~/.config/systemd/user/glass.service`:

```ini
[Unit]
Description=GLASS nightly sync

[Service]
Environment=GL_BASE_URL=https://git.my.com
Environment=DEST_DIR=%h/git.my.com
Environment=CONCURRENCY=8
ExecStart=%h/bin/glass -safe-update -since=168h
Restart=on-failure
```

`~/.config/systemd/user/glass.timer`:

```ini
[Unit]
Description=Run GLASS every night

[Timer]
OnCalendar=*-*-* 23:30:00
Persistent=true

[Install]
WantedBy=timers.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now glass.timer
```

---

## ❓ FAQ

**Q:** Which to choose: working-tree or mirror?
**A:** For backups and CI — `--mirror`. For development — default `-mirror=false`.

**Q:** Will local changes be overwritten?
**A:** No, with `-safe-update` dirty trees are skipped.

**Q:** Some projects didn’t sync?
**A:** Check filters (`-membership`, `-min-access`, `-include/-exclude`, `-since`, `-max-size-mb`).
Also ensure your GitLab PAT has `read_api` and `read_repository`.

**Q:** How to remove local repos after losing access?
**A:** Use `-prune-local`.

---

## 🧩 Architecture

* **fasthttp** — GitLab API (100/page, activity sort, retries).
* **git** — via `exec.CommandContext` with timeout.
* **uber/fx** — DI/lifecycle.
* **zerolog** — structured logging.
* **workers + jobs** — concurrent cloning/updating.

---

## 📜 License

MIT

---

## ✨ Manifest

* **Focus** — run GLASS daily, keep your tree fresh.
* **Fast** — use `-since` and filters to save time.
* **Forward** — new projects and permissions are picked up automatically.

---

**GLASS** — *GitLab Awesome Sync System*

🚀 Let’s ship • 🔥 Fortune favors the bold • 🤝 Respect builds strength
