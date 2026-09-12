# Control Center 0.31 — approval evidence для Change review

## Назначение

Контракт `ui.operations-approval-evidence/v1` формирует ограниченное read-only представление approval policy и фактически пригодных approval records для одной точной immutable revision Change. Это часть закреплённого Roadmap Control Center 0.31.0 — Changes / Jobs operational UI.

Контракт предназначен только для review. Его наличие **не подтверждает право на выполнение**, не переводит Change в `approved`, не создаёт Job, не открывает maintenance window и не заменяет RBAC, policy evaluation, semantic diff, blast radius, preflight, result verification или recovery.

## Точная привязка к Change revision

Evidence обязательно содержит:

- `change_id`;
- точный `revision_id`;
- authoritative SHA-256 digest этой revision;
- `policy_id` и risk level;
- время наблюдения `observed_at`.

Revision digest должен поступать из authoritative revision store. Evidence от другой revision или с другим digest нельзя использовать для approval review текущего Change.

## Что показывает интерфейс

Контракт различает четыре состояния:

- `denied` — policy запрещает Change; approvals не могут превратить deny в allow;
- `not_required` — policy разрешает Change без дополнительных approvals;
- `pending` — требуемого количества пригодных approvals пока нет;
- `satisfied` — policy requirement выполнен.

Для оператора доступны:

- требуемое количество approvals;
- количество записанных approval records;
- количество records, которые реально удовлетворяют policy;
- требования distinct actors / requester prohibition;
- required permission, если она задана policy;
- bounded список пригодных approval records: только actor и `approved_at`.

Полные permission sets approver-ов намеренно не копируются в UI evidence. Контракт показывает результат проверки, а не распространяет authorization material.

## Fail-closed правила

- `change_id`, `revision_id`, requester, `policy_id` и actor identity должны быть непустыми, каноническими и без внешних пробелов;
- revision digest допускается только в виде `sha256:<64 lowercase hex>`;
- максимум 256 approval records; превышение лимита отклоняет evidence целиком, а не обрезает его;
- future-dated approval record отклоняет evidence;
- approval с неканоническим actor не может считаться отдельным approver;
- requester не может удовлетворять policy, если `prohibit_requester=true`;
- один actor учитывается не более одного раза при `distinct_actors=true`;
- approval без required permission не считается пригодным;
- progressed Change (`approved/queued/executing/verifying/succeeded/failed`) не может показываться с неудовлетворённым approval requirement;
- policy deny обязан соответствовать rejected Change state;
- несогласованность между вычисленным evidence и authoritative `policy.CheckApprovals` является ошибкой и не должна превращаться в частичное или оптимистическое отображение.

Дополнительно core approval check подготовлен к fail-closed обработке неканонических requester/actor identity и permission requirement, чтобы пробелы в identity не могли использоваться для обхода requester-prohibition или distinct-actor semantics.

## Security и коммерческая граница

Новый слой не добавляет runtime dependency, SQL migration, third-party redistribution obligation, bundled component или новый license/NOTICE/source-offer requirement. Он не содержит credentials, tokens, revision payload, значения конфигурации и permission arrays approver-ов.

## Граница интеграции

Этот runner-free source slice не меняет `VERSION`, не является release candidate и не должен сам публиковаться как Stable. Он намеренно подготовлен отдельно от NR1 blast-radius и maintenance-window work и не заменяет их.

Следующий runner-зависимый поток должен работать только с точным head этого slice и выполнить один полезный qualification pass без duplicate rerun. После qualification требуется связать approval evidence с authoritative Changes / Jobs provider и RBAC review surface, проверить exact revision/digest binding и отсутствие mutation/execution authority. Только после интеграции с остальными обязательными частями 0.31 и прохождения release gates версия может двигаться к release candidate.
