# GLASS — GitLab Any Sync System

> ⚡ **Коротко:** один бинарь, который подтягивает *все видимые вам проекты GitLab* — быстро, безопасно, с двумя режимами: **mirror** и **working-tree**. Идеален как ежедневный “bootstrap & sync” для разработчиков и как лёгкий бэкап для команд.

---

## Зачем

* 🧭 **Быстрый старт**: одним запуском получаешь всё дерево репозиториев, к которым у тебя есть доступ.
* 🔄 **Ежедневная синхронизация**: подтягивает новые репозитории и обновления по существующим.
* 🛡️ **Безопасность по умолчанию**: аккуратно обновляет рабочие деревья, не ломая локальные изменения.
* 🧰 **Простой DevOps-инструмент**: удобные флаги, понятные логи, можно крутить по cron/systemd.

---

## Возможности (в двух словах)

* **Режимы**:

  * `--mirror` — bare-зеркало (`git clone --mirror`, все ветки/теги/refs).
  * `--mirror=false` — обычный клон с **рабочим деревом**; обновление через `fetch --all --prune`, опц. `checkout` default-ветки.
* **Фильтры**: по namespace (regexp), по активности `--since`, по размеру `--max-size-mb`.
* **Протокол**: HTTPS + PAT (по умолчанию) или `--ssh` для SSH-URL.
* **Безопасные апдейты**: `--safe-update` (skip, если дерево грязное), `--force-reset` (опционально, с пониманием риска).
* **Прочее**: пагинация 100/страница, ретраи + backoff, `--prune-local` (удаление локальных “сирот”), сабмодули.

---

## Установка

```bash
go version  # требуется Go 1.24+
git clone <ваш-репо-с-glass>
cd glass
go build -o glass .
```

---

## Быстрый старт (FFF)

1. Создай PAT в GitLab с `read_api` + `read_repository`.
2. Выбери папку назначения, например `~/glass-root`.
3. Запусти:

```bash
export GL_BASE_URL="https://gitlab.my.company"
export GL_TOKEN="glpat_xxx"
export DEST_DIR="$HOME/glass-root"
export CONCURRENCY=8
# Режим рабочего дерева (рекомендуется девам):
./glass -mirror=false -recurse-submodules -checkout-default
```

> Хочешь зеркала для бэкапа? Используй `./glass -mirror`.

---

## Типовые сценарии

### A. Ежедневный dev-sync (рекомендуется)

```bash
./glass \
  -mirror=false \
  -recurse-submodules \
  -checkout-default \
  -safe-update \
  -since=168h \
  -j 8
```

* Обновит активные за 7 дней репозитории; грязные рабочие деревья **не тронет**.

### B. Полный бэкап-зеркало

```bash
./glass \
  -mirror \
  -archived=false \
  -prune-local \
  -j 8
```

* Получишь bare-зеркала всех доступных проектов, удалит локальные каталоги, к которым доступ потерян.

### C. Только мой namespace

```bash
./glass \
  -mirror=false \
  -include='^team-x/|^products/a7a5-' \
  -exclude='/legacy-' \
  -j 6
```

### D. SSH вместо PAT в URL

```bash
./glass -mirror=false -ssh
```

> Подразумевает настроенные SSH-ключи к GitLab.

---

## Флаги

| Флаг                  | Описание                                                     | По умолчанию      |
| --------------------- | ------------------------------------------------------------ | ----------------- |
| `-base-url`           | База GitLab (`https://gitlab.com`/self-hosted)               | —                 |
| `-token`              | Personal Access Token (PAT) с `read_api` + `read_repository` | —                 |
| `-dest`               | Корневая папка назначения                                    | `./gitlab-backup` |
| `-j`                  | Конкурентность (workers)                                     | `4`               |
| `-dry-run`            | Показывать, что будет сделано, без действий                  | `false`           |
| `-membership`         | Только проекты, где вы участник                              | `true`            |
| `-min-access`         | Мин. уровень доступа (10..50)                                | `10`              |
| `-archived`           | Включать архивные проекты                                    | `false`           |
| `-http-verbose`       | Логировать HTTP-запросы                                      | `false`           |
| `-timeout`            | Таймаут HTTP на запрос                                       | `30s`             |
| `-mirror`             | Режим `git clone --mirror`                                   | `true`            |
| `-recurse-submodules` | Рекурсивные сабмодули (для non-mirror)                       | `false`           |
| `-checkout-default`   | `checkout` default-ветки после fetch                         | `true`            |
| `-ssh`                | Использовать SSH-URL вместо HTTPS+PAT                        | `false`           |
| `-safe-update`        | Пропустить checkout/pull при грязном дереве                  | `true`            |
| `-force-reset`        | `reset --hard` на `origin/<default>` (опасно)                | `false`           |
| `-include`            | Regexp-фильтр include по `path_with_namespace`               | —                 |
| `-exclude`            | Regexp-фильтр exclude по `path_with_namespace`               | —                 |
| `-since`              | Брать проекты активные за период (`72h`, `7d`)               | `0` (все)         |
| `-max-size-mb`        | Пропускать проекты больше N МБ                               | `0` (выкл)        |
| `-prune-local`        | Удалять локальные репы, которых нет в API                    | `false`           |
| `-git-timeout`        | Таймаут на одну `git`-команду                                | `10m`             |

**ENV-аналоги**: `GL_BASE_URL`, `GL_TOKEN`, `DEST_DIR`, `CONCURRENCY`, `DRY_RUN`, `MEMBERSHIP`, `MIN_ACCESS`, `INCLUDE_ARCHIVED`, `HTTP_VERBOSE`, `TIMEOUT`, `MIRROR`, `RECURSE_SUBMODULES`, `CHECKOUT_DEFAULT`, `SSH_MODE`, `SAFE_UPDATE`, `FORCE_RESET`, `INCLUDE`, `EXCLUDE`, `SINCE`, `MAX_SIZE_MB`, `PRUNE_LOCAL`, `GIT_TIMEOUT`, `DEBUG=1` (повышает лог-уровень).

---

## Поведение и безопасность

* **HTTPS+PAT**: токен внедряется как `https://oauth2:<TOKEN>@…` **только** в git-команду. В логи токен не попадает.
* **SSH-режим**: `-ssh` использует `SSHURLToRepo` и ваши ключи; удобно для персональных машин.
* **Safe Update**: при грязном рабочем дереве GLASS не делает `checkout/pull` — только `fetch`.
* **Force Reset**: включать осознанно (CI/бэкап-агенты), в дев-профиле обычно не нужен.

---

## Логи и отчётность

* Консольные логи в стиле `zerolog`, хорошо парсятся.
* Каждые 2с выводится прогресс: `total / in_progress / done`.
* При желании включайте `DEBUG=1` — больше деталей (HTTP-URLs без токена, `git` stdout/stderr усечены до 8KB).
* По завершению — сводка (cloned/updated/skipped), если включено в сборке.

---

## Интеграция с планировщиком

### systemd (ежевечерний sync)

`~/.config/systemd/user/glass.service`

```ini
[Unit]
Description=GLASS nightly sync

[Service]
Environment=GL_BASE_URL=https://gitlab.my.company
Environment=GL_TOKEN=glpat_xxx
Environment=DEST_DIR=%h/glass-root
Environment=CONCURRENCY=8
ExecStart=%h/bin/glass -mirror=false -recurse-submodules -checkout-default -safe-update -since=168h
Nice=10
IOSchedulingClass=best-effort
Restart=on-failure
```

`~/.config/systemd/user/glass.timer`

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

## Частые вопросы

**Q:** Что выбрать: mirror или working-tree?
**A:** Для бэкапов и CI-зеркал — **mirror**. Для ежедневной разработки — **working-tree** (`-mirror=false`).

**Q:** У меня локальные правки — их перетрёт?
**A:** Нет, в дефолте включён `-safe-update`. При грязном дереве GLASS не делает `checkout/pull`.

**Q:** Почему некоторые репозитории не подтянулись?
**A:** Проверь фильтры (`-membership`, `-min-access`, `-include/-exclude`, `-since`, `-max-size-mb`).
Также убедись, что токен имеет `read_api` и `read_repository`.

**Q:** Можно ли чистить локальные репы, если доступ сняли?
**A:** Да, `-prune-local` удалит “сирот”.

---

## Траoubleshooting

* **HTTP 429 / 5xx**: вшиты ретраи с backoff + `Retry-After`. Если сеть совсем злая — подними `-timeout`.
* **git зависает**: выставь меньший `-git-timeout` и смотри логи по конкретной команде.
* **Сабмодули не подтянулись**: добавь `-recurse-submodules` (только для non-mirror).

---

## Архитектура (в одном абзаце)

* **fasthttp** для GitLab API (страницы по 100, сортировка по активности, ретраи).
* **git** через `exec.CommandContext` с таймаутом; аккуратные логи.
* **fx** для DI/жизненного цикла; единый `main.go`, минимум зависимостей.
* **Модель “tasks + workers”** для параллельной обработки репозиториев.

---

## Лицензия

MIT (или укажи свою корпоративную).

---

## Манифест команды (FFF)

* **Focus** — запускай GLASS ежедневно, держи локальное дерево в актуальном состоянии.
* **Fast** — работай с рабочим деревом, `fetch` быстрый, `since` и фильтры экономят время.
* **Forward** — новые права/проекты подхватываются автоматически; вечером синканул — утром уже работаешь.

---

**Имя и слоган:** **GLASS** — *GitLab Any Sync System*.
“Let’s ship 🚀 • Fortune favors the bold ✨ • Respect builds strength 🤝”
