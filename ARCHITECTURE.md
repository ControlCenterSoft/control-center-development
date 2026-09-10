# Архитектура Control Center

Этот документ фиксирует продуктовую архитектуру и обязательные эксплуатационные инварианты Control Center. Он описывает как уже опубликованные основы, так и целевую модель. Конкретная возможность считается доступной только тогда, когда она фактически реализована и опубликована в соответствующей версии; наличие архитектурного контракта само по себе не является обещанием production-доступности.

На 10.09.2026 последний опубликованный исходный релиз — **0.16.0**. Отдельный стабильный бинарный канал распространения подтверждён до **0.3.1**.

## 1. Базовые принципы

Control Center строится как модульная инфраструктурная платформа с жёсткими доменными границами и типизированными операциями. Вынос компонента в отдельный процесс или узел оправдан только измеренной нагрузкой, независимым жизненным циклом, требованиями отказоустойчивости либо отдельной границей доверия.

Канонический путь изменения состояния:

`Запрос → Валидация → Авторизация → План → Change → Job → типизированное действие → Проверка → Actual State → Audit`

Обязательные инварианты:

- RBAC работает deny-by-default и проверяется на сервере;
- Web/API не предоставляет произвольный shell/exec как продуктовую функцию;
- длительные и опасные изменения выполняются через Change/Job;
- Jobs имеют явную модель idempotency, retries, leases и завершения;
- Desired State отделён от Actual State;
- для опасных операций определяются preflight, blast radius, проверка результата и recovery/rollback;
- транзакционное состояние хранится отдельно от тяжёлой телеметрии, больших артефактов и резервных копий;
- автоматизация не получает полномочий шире, чем разрешают RBAC, policy и конкретный типизированный action.

## 2. Core и Market

### Core

Core содержит обязательные платформенные подсистемы и общие границы безопасности:

- Identity, Sessions и RBAC;
- Desired State / Actual State;
- Change и durable Job execution;
- Nodes, Roles и Lifecycle;
- Site/Zone/Scope foundation;
- Network Management;
- Monitoring, Health и Audit;
- Backup/Recovery contracts;
- Capacity Planner / Capacity Intelligence foundation;
- общие API, persistence и security contracts.

### Market

Market содержит устанавливаемые инфраструктурные возможности. Market-модуль не становится частью Core только потому, что использует его API.

Для поддерживаемого модуля должны быть определены как минимум:

- устойчивый module identity;
- compatibility, dependencies и conflicts;
- требуемые permissions/capabilities;
- network и storage requirements;
- capacity profile;
- install/update/migrate/backup/restore/remove lifecycle;
- health и audit semantics;
- failure/recovery model.

К направлениям Market относятся Directory Services, DNS/DHCP, PXE Windows/Linux, Software Automation Windows/Linux, Inventory/Compliance, File Services, Monitoring, Backup и другие инфраструктурные providers. Наличие направления в архитектуре или roadmap не означает, что соответствующий модуль уже опубликован.

## 3. Desired State и Actual State

Desired State описывает намерение администратора. Actual State описывает наблюдаемое фактическое состояние. Эти модели не должны подменять друг друга.

Каждый управляемый объект должен иметь достаточную identity/version metadata, чтобы система могла обнаруживать stale-state, concurrent update и drift. Изменение Desired State не считается завершённым, пока соответствующий Job не выполнился и Actual State/Health не подтвердили требуемый результат.

Reconciliation допускается только в пределах явно разрешённого контракта. Автоматический reconciler не должен выполнять произвольные destructive actions из одного лишь факта расхождения Desired/Actual.

## 4. Change и Job execution

Опасные или длительные операции оформляются как Changes и durable Jobs. План должен быть понятен до исполнения и фиксировать хотя бы:

- что изменится;
- целевые объекты и scope;
- риск и blast radius;
- необходимые preconditions;
- ожидаемый результат;
- метод проверки;
- recovery/rollback либо безопасную compensation модель.

Worker выполняет только allowlisted typed actions. Повтор Job после сбоя не должен превращать идемпотентную операцию в повторное destructive изменение.

## 5. Узлы, роли и lifecycle

Узел является first-class объектом с identity, site/zone, hardware/network inventory, assigned roles, health и lifecycle state.

Целевая ролевая модель допускает специализированные роли управления, исполнения, хранения данных, consensus, telemetry, backup/repository и edge-функции. Конкретный набор поддерживаемых ролей определяется опубликованной версией.

Lifecycle узла включает контролируемые переходы, в том числе:

`enrollment → active → maintenance → drain → replacement/decommission`

Maintenance не должен автоматически означать удаление данных. Drain обязан учитывать stateful workloads и provider-specific ограничения. Replacement должен сохранять identity/ownership semantics там, где это необходимо. Decommission допускается только после проверки зависимостей, данных, recovery path и отсутствия незавершённых Jobs.

## 6. Single-node и multi-node/HA

Single-node — полноценный режим использования Control Center. Он не должен искусственно требовать второй сервер.

Multi-node модель предназначена для распределения ролей, ёмкости и отказоустойчивости. Целевая HA-модель включает controller membership, quorum/consensus, fencing/split-brain protection, controlled switchover/failover, восстановление после потери узлов и HA-профили stateful данных.

Наличие multi-node объектов или HA-контрактов не означает, что production failover уже поддерживается. HA считается поддержанным только для конкретных ролей и версий, для которых опубликованы и проверены quorum/fencing/failover/recovery процедуры.

## 7. Управляемая сеть

Network Management является частью Core и рассматривается как отдельный безопасный subsystem, а не набор произвольных команд ОС.

Целевая модель поддерживает:

- multi-NIC;
- назначение интерфейсам ролей/зон, включая типовые WAN/LAN;
- VLAN и bonding там, где это поддерживается платформой;
- routing;
- DNS/NTP;
- firewall policy;
- NAT/port-forwarding только при явном включении;
- staged changes с preflight, connectivity verification и rollback.

Наличие WAN+LAN **не делает узел маршрутизатором автоматически** и не включает forwarding/NAT/port-forwarding. Сетевое изменение не должно оставлять узел недоступным без заранее определённого recovery path.

Изменения IP, routing, firewall и management connectivity относятся к повышенному риску и должны применяться поэтапно с проверкой новой связности до окончательной фиксации.

## 8. Capacity Planner и Capacity Intelligence

Capacity Planner использует hardware inventory, наблюдаемую загрузку, storage/network characteristics, workload profiles и исторические данные для оценки безопасной ёмкости и bottlenecks.

Целевая модель предусматривает:

- безопасный запас CPU/RAM/storage/network;
- bottleneck identification;
- workload forecast;
- horizon до исчерпания резерва;
- what-if planning;
- placement/scale recommendations;
- calibration по измеренному evidence;
- анализ эффективности масштабирования и сценариев изменения нагрузки.

Опубликованная линия 0.7–0.16 развивает этот контур как **advisory-only**. В 0.15 опубликована bounded nonlinear Workload Curve с piecewise interpolation только внутри измеренного диапазона и запретом extrapolation; в 0.16 опубликован Workload Curve Efficiency для обнаружения diminishing returns с fail-closed проверкой malformed, non-increasing и non-finite evidence.

Capacity evidence и рекомендации не являются разрешением на production mutation. Они не должны самостоятельно выполнять placement, drain, migration, resize, rebalance, изменение сети или закупку ресурсов.

## 9. Persistence и данные

PostgreSQL используется для транзакционного состояния Control Center в поддерживаемых профилях. Изменения схемы должны быть версионированы, воспроизводимы и иметь понятный forward migration path.

Перед потенциально опасным обновлением базы требуется проверенный backup/recovery path. Приложение не должно считать миграцию успешной только потому, что процесс завершился без ошибки: после изменения необходимы schema/runtime readiness checks.

Большие бинарные артефакты, долговременные резервные копии и высокочастотная телеметрия должны иметь подходящие специализированные хранилища и retention policy, а не бесконтрольно расти в основной транзакционной БД.

## 10. Backup и Recovery

Recovery — отдельная продуктовая способность, а не побочный эффект наличия backup-файла.

Целевая модель включает Recovery Point metadata, backup integrity/retention, restore verification, PostgreSQL backup/PITR для поддерживаемых профилей, object-level recovery там, где это безопасно, RPO/RTO policies, restore drills и восстановление после отказа узла либо ошибочных действий оператора/приложения.

Backup считается полезным только при наличии проверяемого restore path.

## 11. Monitoring, Health и Audit

Health разделяется как минимум на liveness и readiness. Компонент может быть жив, но не готов обслуживать запросы из-за базы данных, миграций, quorum, storage или иной зависимости.

Audit должен фиксировать security- и state-changing события с достаточной identity/context metadata. История аудита не должна превращаться в скрытый канал хранения секретов.

Observability обязана помогать ответить: что произошло, кто инициировал, какой план выполнялся, какой Job исполнялся, какой Actual State получен и какой recovery path доступен.

## 12. Authentication policy

Для опубликованной исходной линии начиная с 0.6.0 действует локальная bootstrap-политика: после чистой установки создаётся пользователь `admin` с первоначальным паролем `admin`.

При первом входе пользователь обязан сменить пароль; до смены обычная работа с системой запрещена. При последующем обновлении существующий пользовательский пароль сохраняется и не сбрасывается к первоначальному значению.

Отдельный stable-binary выпуск 0.3.1 относится к более ранней bootstrap-модели, поэтому его эксплуатационное поведение должно определяться документацией именно этого бинарного выпуска.

## 13. Upgrade safety

Обновление Control Center должно быть version-aware и recovery-aware. Для stateful профиля безопасный порядок включает:

1. проверку текущей и целевой version identity;
2. compatibility/preflight;
3. проверку свободного места и зависимостей;
4. backup и проверку rollback artifact;
5. forward migrations;
6. переключение runtime на новую версию;
7. restart;
8. liveness/readiness;
9. version/login/RBAC/API/UI smoke;
10. rollback/recovery при деградации.

Для multi-node/HA обновление должно учитывать quorum, порядок узлов, drain/maintenance и допустимое число одновременно недоступных экземпляров.

## 14. Security boundaries

Control Center не должен превращать удобство автоматизации в универсальный удалённый shell. Привилегированные операции должны быть минимальными, типизированными, scoped и аудируемыми.

Секреты не должны попадать в API-ответы, логи, audit payloads или пользовательскую документацию. Чувствительные credentials должны храниться и передаваться через предназначенные для этого механизмы.

Network, backup, restore, identity, role assignment и destructive lifecycle operations требуют повышенного уровня проверки и явного recovery plan.

## 15. Граница опубликованной функциональности

Последний опубликованный исходный релиз 0.16.0 подтверждает развитие Capacity Intelligence до Workload Curve Efficiency. Более новые capability stages должны считаться предварительными до официальной публикации соответствующих версий.

Отдельный stable binary channel имеет собственную release identity. Более новый исходный релиз нельзя описывать как уже доступный бинарный stable-пакет, пока соответствующий пакет фактически не опубликован.
