# 🚀 GLASS — GitLab Any Sync System

> ⚡ **Коротко:** один бинарь, который подтягивает *все видимые вам проекты GitLab* — быстро, безопасно, с двумя режимами: **working-tree** и **mirror**.  
> Идеален как ежедневный “bootstrap & sync” для разработчиков и как лёгкий бэкап для команд.

---

## 🧭 Зачем

* **Быстрый старт** — одним запуском получаешь всё дерево репозиториев, к которым у тебя есть доступ.
* **Ежедневная синхронизация** — подтягивает новые репозитории и изменения коллег.
* **Безопасность по умолчанию** — аккуратно обновляет рабочие деревья, не ломая локальные изменения.
* **Простой DevOps-инструмент** — удобные флаги, понятные логи, можно крутить по cron/systemd.

---

## 🔧 Возможности

* **Режимы**:
  * `--mirror` — bare-зеркало для DevOps (`git clone --mirror`, все ветки/теги/refs).
  * `--mirror=false` — по умолчанию, обычный клон с рабочим деревом; обновление через `fetch --all --prune`, опц. `checkout` default-ветки.
* **Фильтры**: по namespace (regexp), по активности (`--since`), по размеру (`--max-size-mb`).
* **Протоколы**: HTTPS+PAT (по умолчанию) или `--ssh` для SSH-URL.
* **Безопасные апдейты**: `--safe-update` (skip, если дерево грязное), `--force-reset` (жёсткий reset).
* **Прочее**: пагинация 100/страница, ретраи+backoff, `--prune-local` (удаление “сирот”), поддержка сабмодулей.

---

## 📥 Установка

Требуется: `git`, `go >= 1.24`, `curl` или `wget`.

```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/xakepp35/glass/main/install.sh)"
```

или:

```bash
sh -c "$(wget -qO- https://raw.githubusercontent.com/xakepp35/glass/main/install.sh)"
```

Бинарь устанавливается в `/usr/local/bin/glass`.

---

## ⚡ Быстрый старт (FFF)

1. Настрой `~/.netrc`:

   ```text
   machine git.my.com
   login oauth2
   password glpat-xxxxxxxxxxxx
   ```

   👉 Важно: `chmod 600 ~/.netrc`

2. Добавь в `~/.bashrc` или `~/.zshrc`:

   ```bash
   export GL_BASE_URL="https://git.my.com"
   export DEST_DIR="$HOME/git.my.com"
   ```

   затем обнови сессию:

   ```bash
   source ~/.bashrc   # или source ~/.zshrc
   ```

3. Запусти:

   ```bash
   glass
   ```

👉 Все проекты подтянутся в `~/git.my.com`.

---

## 🛠 Использование

| Роль              | Команда                                                                 | Комментарий                            |
| ----------------- | ----------------------------------------------------------------------- | -------------------------------------- |
| 👶 Новый dev      | `glass -mirror=false -checkout-default -j 4`                            | bootstrap всех доступных репозиториев  |
| 👨‍💻 Разработчик | `glass -mirror=false -safe-update -since=168h -j 8`                     | ежедневный sync активных за неделю реп |
| 🧑‍🤝‍🧑 Тимлид   | `glass -mirror=false -checkout-default -archived -j 12`                 | полный sync, включая архивные          |
| 🔒 SRE/DevOps     | `glass -mirror -prune-local -archived -j 16`                            | nightly mirror-бэкап                   |
| 🧪 QA             | `glass -mirror=false -include='^qa-projects/' -recurse-submodules -j 4` | только тестовые проекты с сабмодулями  |
| SSH вместо PAT    | `glass -mirror=false -ssh`                                              | git по SSH, API по PAT в netrc         |
| Только namespace  | `glass -mirror=false -include='^team-x/' -exclude='/legacy-' -j 6`      | выборочно                              |

---

## ⚙️ Флаги и переменные окружения

| Флаг                  | ENV аналог           | Описание                                                     | По умолчанию      |
| --------------------- | -------------------- | ------------------------------------------------------------ | ----------------- |
| `-base-url`           | `GL_BASE_URL`        | База GitLab (`https://gitlab.com`/self-hosted)               | —                 |
| `-token`              | `GL_TOKEN`           | Personal Access Token (PAT) с `read_api` + `read_repository` | —                 |
| `-dest`               | `DEST_DIR`           | Корневая папка назначения                                    | `./gitlab-backup` |
| `-j`                  | `CONCURRENCY`        | Конкурентность (workers)                                     | `4`               |
| `-dry-run`            | `DRY_RUN`            | Показывать, что будет сделано, без действий                  | `false`           |
| `-membership`         | `MEMBERSHIP`         | Только проекты, где вы участник                              | `true`            |
| `-min-access`         | `MIN_ACCESS`         | Мин. уровень доступа (10..50)                                | `10`              |
| `-archived`           | `INCLUDE_ARCHIVED`   | Включать архивные проекты                                    | `false`           |
| `-http-verbose`       | `HTTP_VERBOSE`       | Логировать HTTP-запросы                                      | `false`           |
| `-timeout`            | `TIMEOUT`            | Таймаут HTTP на запрос                                       | `30s`             |
| `-mirror`             | `MIRROR`             | Режим `git clone --mirror`                                   | `false`           |
| `-recurse-submodules` | `RECURSE_SUBMODULES` | Рекурсивные сабмодули (для non-mirror)                       | `false`           |
| `-checkout-default`   | `CHECKOUT_DEFAULT`   | `checkout` default-ветки после fetch                         | `true`            |
| `-ssh`                | `SSH_MODE`           | Использовать SSH-URL вместо HTTPS+PAT                        | `false`           |
| `-safe-update`        | `SAFE_UPDATE`        | Пропустить checkout/pull при грязном дереве                  | `true`            |
| `-force-reset`        | `FORCE_RESET`        | `reset --hard` на `origin/<default>` (опасно)                | `false`           |
| `-include`            | `INCLUDE`            | Regexp-фильтр include по `path_with_namespace`               | —                 |
| `-exclude`            | `EXCLUDE`            | Regexp-фильтр exclude по `path_with_namespace`               | —                 |
| `-since`              | `SINCE`              | Брать проекты активные за период (`72h`, `7d`)               | `0` (все)         |
| `-max-size-mb`        | `MAX_SIZE_MB`        | Пропускать проекты больше N МБ                               | `0` (выкл)        |
| `-prune-local`        | `PRUNE_LOCAL`        | Удалять локальные репы, которых нет в API                    | `false`           |
| `-git-timeout`        | `GIT_TIMEOUT`        | Таймаут на одну `git`-команду                                | `10m`             |
| `-use-netrc-api`      | `USE_NETRC_API`      | Читать PAT для API из `~/.netrc`, если `-token` пуст         | `true`            |
| `-use-netrc-git`      | `USE_NETRC_GIT`      | Не встраивать PAT в HTTPS-URL, git берёт креды из `.netrc`   | `true`            |
| `-netrc`              | `NETRC`              | Путь к `.netrc`                                              | `~/.netrc`        |

---

## 🔒 Поведение и безопасность

* **HTTPS+PAT**: токен внедряется в git-URL только в момент запуска, в логи не попадает.
* **SSH-режим**: использует `SSHURLToRepo` и ваши ключи.
* **Safe Update**: при грязном дереве GLASS не делает `checkout/pull` — только `fetch`.
* **Force Reset**: использовать только для CI/бэкапов.

---

## ⏰ Интеграция с systemd

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

**Q:** Что выбрать: working-tree или mirror?
**A:** Для бэкапов и CI — `--mirror`. Для разработки по умолчанию — `-mirror=false`.

**Q:** Локальные правки не затрутся?
**A:** Нет, при `-safe-update` грязные деревья не трогаются.

**Q:** Некоторые проекты не подтянулись?
**A:** Проверь фильтры (`-membership`, `-min-access`, `-include/-exclude`, `-since`, `-max-size-mb`).
Также убедись, что GitLab PAT токен имеет `read_api` и `read_repository`.

**Q:** Как чистить локальные репы, к которым доступ сняли?
**A:** Флаг `-prune-local`.

---

## 🧩 Архитектура

* **fasthttp** — GitLab API (100/page, сортировка по активности, ретраи).
* **git** — через `exec.CommandContext` с таймаутом.
* **uber/fx** — DI/жизненный цикл.
* **zerolog** — структурированные логи.
* **workers + jobs** — конкурентный клон/апдейт.

---

## 📜 Лицензия

MIT

---

## ✨ Манифест

* **Focus** — запускай GLASS ежедневно, держи дерево в актуальном состоянии.
* **Fast** — используй `-since` и фильтры, экономь время.
* **Forward** — новые проекты и доступы подтянутся автоматически.

---

**GLASS** — *GitLab Any Sync System*

🚀 Let’s ship • 🔥 Fortune favors the bold • 🤝 Respect builds strength
