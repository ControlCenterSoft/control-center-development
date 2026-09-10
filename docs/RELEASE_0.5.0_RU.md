# Control Center 0.5.0

Релиз 0.5.0 закрывает этап Multi-node Operations и Lifecycle поверх распределённых контрактов 0.4.

В релиз входят:

- one-time bootstrap grants и каноническое enrollment admission с roles, scope, site и management zone;
- безопасные Drain/Replace plans со строгой границей stateless/stateful;
- canary-first Upgrade Orchestrator с dependency graph, update rings, maintenance window, quorum boundary, rollback и health gates;
- Placement Planner v1: детерминированный выбор по dominant utilization, topology/version preconditions, affinity, roles, site/zone и capacity;
- Capacity Planner MVP: safe capacity с N-node failure reserve, bottleneck, confidence и advisory action;
- синтетическая модель нагрузки агентов с проверкой stale-result rejection и отсутствия false Success.

Все планы требуют разрешённого Change, durable Job, Audit и повторной проверки предусловий перед исполнением. Планировщики сами по себе не изменяют управляемые узлы и не дают полномочий на произвольную production mutation.

0.5.0 опубликован как исходный релиз. Фактическая доступность устанавливаемого бинарного дистрибутива определяется отдельно опубликованным каналом распространения конкретной версии.
