# 🛣 GLASS Roadmap

> **GLASS — GitLab Any Sync System.**  
> Сегодня это утилита для массового sync/backup GitLab-репозиториев.  
> Завтра — единый «оркестратор многорепных операций» для разработчиков и команд.  
> Делаем просто. Делаем системно. Делаем правильно.  

---

## 🎯 Цель

*Универсальный инструмент управления множеством Git-репозиториев.*  
Не заменитель `git`, а надстройка «git-of-gits»:  
- bootstrap & sync для разработчиков,  
- backup & inventory для лидов и SRE,  
- контролируемые write-операции для команд.

---

## ⚡ Фокус

1. **Read-first**: надёжный sync, bootstrap, mirror.  
2. **Stateful**: у каждого запуска есть память и история (`.glass/`).  
3. **CLI-глаголы**: `glass sync/list/status/report/prune/export/stash/restore/...`.  
4. **Safe write**: stash, branch, tag, publish — но только через безопасные рельсы.  
5. **Policy-as-code**: правила команд/организаций проверяются перед действием.  
6. **Reports**: JSON/Markdown/CSV инвентарь, видимость и контроль.  

---

## 📂 Архитектура состояния

В `DEST_DIR/.glass/`:

```
state.json           # текущее состояние (последний sync)
runs/*.jsonl         # логи запусков (JSONL)
reports/*.json|.md   # инвентари, сводки
tx/<id>/             # транзакции write-операций (plan/log/result)
locks/\*.lock         # защита от параллельных запусков
```

*Просто, читаемо, парсится `jq`, поддерживает аудит и анализ.*

---

## 🕹 Глаголы CLI

- `glass sync` — базовый sync/mirror.  
- `glass list` — список проектов (из API или кэша).  
- `glass status` — сводка по последнему запуску.  
- `glass report` — генерирует инвентарь и summary.  
- `glass prune` — чистка «сирот».  
- `glass export` — tar.gz снапшот дерева.  
- `glass stash` — массовый stash (локальный/remote).  
- `glass restore` — восстановление снапшота.  
- `glass standardize` — привести все репы к default-ветке.  
- `glass branch` — согласованное создание веток.  
- `glass tag` — массовое тегирование.  
- `glass publish` — безопасный push (FF-only, MR-fallback).  
- `glass tx *` — работа с транзакциями (list/show/resume/abort).  

---

## 🔒 Безопасность

- Dry-run по умолчанию, `--confirm` для выполнения.  
- Protected ветки — только через MR.  
- FF-only — дефолт, никаких `push -f`.  
- Policy-файл `.glasspolicy.yaml`:  
  - allow/deny targets,  
  - MR-правила,  
  - branch-prefix и пр.  
- Журнал транзакций для аудита и повторных прогонов.

---

## 📊 Отчётность

- **JSONL логи** — метрики без метрик-сервера.  
- **state.json** — сводка «последний run».  
- **inventory.json / summary.md** — инвентарь для лидов/архитекторов.  
- Парсится `jq`, интегрируется в CI, загружается в Prometheus/ELK по желанию.  

---

## 🛡 Направления развития

### 0. Нормализация и стандартизация флагов и энвов, введение глаголов
- `glass sync`
- GLASS_DIR, GLASS_URL итп

### 1. Жизненный цикл и чистка
- Retention для `.glass/runs` и `.glass/reports`.  
- Auto-prune по времени/размеру.  
- Политики retention для зеркал.  

### 2. Инвентаризация
- Расширенные отчёты (размер, активность, доступы).  
- Генерация CSV/Markdown для менеджмента.  

### 3. Security / Compliance
- Интеграция с Secret Managers (Vault, AWS/GCP).  
- Проверки: нет публичных форков, нет «лишних» проектов.  

### 4. Multi-provider
- GitLab (сейчас), затем GitHub/Gitea/Bitbucket.  
- Единый интерфейс `-provider`.  

### 5. Team bootstrap
- `glass export` для снапшотов → раздавать разработчикам.  
- Быстрый старт целых команд.  

### 6. Observability
- Простой `--logfmt` для интеграции в Promtail/Vector.  
- Опционально `glass serve --metrics` для Prometheus.  

---

## 🚀 Мотивация

- **Focus**: простота, read-first, безопасные write.  
- **Fast**: bootstrap/sync/report мгновенно доступны каждому.  
- **Forward**: из dev-хелпера в корпоративный оркестратор многорепных операций.

---

## 🧭 Дорожная карта

1. **v1.0** — Sync/backup (read-only, как сейчас).  
2. **v1.1** — `.glass/` состояние, `status`, `report`, `list`.  
3. **v1.2** — `prune`, `export`, retention.  
4. **v1.3** — write-операции: `stash`, `standardize`, `branch`, `tag`, `publish`.  
5. **v1.4** — транзакции (`tx`-журнал, resume/abort).  
6. **v1.5** — policies (`.glasspolicy.yaml`).  
7. **v2.x** — multi-provider, observability, secrets integration.  

---

**GLASS** — *Git Awesome Sync System*
