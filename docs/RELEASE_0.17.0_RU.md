# Control Center 0.17.0

Статус: активная ветка разработки, не официальный релиз.

Версия 0.17 продолжает nonlinear Workload Curve 0.15 и Efficiency Report 0.16, добавляя bounded scale-scenario evaluation. Новый контур отвечает на вопрос, помещается ли заданная плановая нагрузка с требуемым headroom на конкретном resource factor, но только внутри реально измеренной benchmark-кривой.

## Workload Scale Scenario

`internal/capacity/workload_scale_scenario.go` принимает exact Workload Curve, exact Efficiency Report и запрос с target resource factor, required workload и минимальным headroom.

Перед расчётом Efficiency Report полностью пересобирается из исходной кривой и policy. Любое изменение report fields, ID, status или reason отклоняется как evidence mismatch.

Сценарий:

- использует `EstimateWorkloadCurve()` и поэтому не выполняет extrapolation за пределами измеренного диапазона;
- рассчитывает доступную safe workload и фактический headroom;
- возвращает `insufficient-capacity`, если требуемый резерв не выдерживается;
- сохраняет предупреждение `diminishing-returns`, даже когда абсолютная ёмкость достаточна;
- возвращает `collect-evidence` при недостаточном или вне диапазона evidence;
- fail-closed блокирует неподтверждённые входные данные.

## Границы безопасности

Scale Scenario остаётся аналитическим evidence:

- `advisory_only = true`;
- `production_mutation = false`;
- не выполняет resize, placement, drain, migration или rebalance;
- не меняет Desired State или Actual State;
- не изменяет сеть, storage или workload policy;
- не запускает provider-команды;
- не создаёт разрешение на закупку или автоматическое масштабирование.

Добавлены проверки безопасного сценария, недостаточного headroom, сохранения diminishing-returns warning, запрета extrapolation и tamper detection для exact Efficiency Report. JSON contract закрыт для неизвестных полей.

Официальный выпуск 0.17.0 допускается только после отдельной qualification и release gates.
