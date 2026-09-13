# Control Center — политика оркестрации разработки и выпуска

Статус: **CURRENT / NORMATIVE FOR DEVELOPMENT OPERATIONS**

Основной язык документации — русский. Английский используется только там, где он необходим для API/идентификаторов, кода, команд, протоколов и стандартов, официальных названий внешних технологий и английской UI-локализации.

## 1. Ресурсная политика

Общий development/runner capacity распределяется по фактической активной нагрузке:

- Control Center — **60%**;
- Home Center — **30%**;
- Website / Client Portal / Admin Portal — **10%**.

Количество задач не обязано совпадать с процентами. Свободная квота может временно использоваться другим потоком, но возвращается владельцу на ближайшей 10-минутной точке при наличии готовой очереди.

## 2. Почасовая оркестрация

Допускается до 15 параллельных рабочих задач. Диспетчеризация выполняется по точкам `*:00`, `*:10`, `*:20`, `*:30`, `*:40`, `*:50`. Долгие операции не прерываются искусственно на границе 10 минут; точки используются для запуска нового полезного slice, перераспределения ёмкости, review, qualification и release work.

В `*:00` формируется краткий delta-отчёт простым языком с акцентом на то, что конкретно изменилось за последний час: функциональный результат, Stable/release progress, site/LK sync, документация, Gemini, блокеры и следующий ожидаемый результат.

## 3. Anti-drift policy

Контролируются четыре независимых вида расхождения:

1. **Code Drift** — development далеко опережает ближайший реально выпускаемый release train;
2. **Release Drift** — qualified/main далеко опережают Stable branch/release;
3. **Production/Site Drift** — опубликованный Stable или qualified portal source не отражены в применимом production-сайте/ЛК;
4. **Documentation Drift** — Google Drive, repository-side docs, фактический код, Stable docs и production claims противоречат друг другу.

Режимы:

- `NORMAL` — дистанция контролируема;
- `DRIFT WARNING` — невыпущенный scope растёт, новые feature slices ограничиваются;
- `RELEASE RESCUE` — 70–80% собственной квоты проблемного потока временно направляется на integration, regression/security, qualification, Stable promotion, site/LK sync и документационное выравнивание.

Ближайший release имеет приоритет над будущим scope. Ранняя 0.43 foundation-разработка допустима только в пределах замороженной архитектуры и не должна останавливать последовательную release-линейку 0.32–0.42.

## 4. Полный Definition of Done

Существенный slice не считается `DONE` на merge в main. Полная цепочка:

`актуальный RoadMap/ТЗ → implementation → tests/security → Gemini review → integration → GitHub docs sync → Google Drive docs sync → release qualification → Stable branch/release → site/LK sync при применимости → production verification`.

Если Gemini два раза подряд технически не может выполнить review, фиксируется `SKIPPED_TECHNICAL_FAILURE` с причиной и разработка продолжается. Реальные `BLOCKER`/`HIGH` findings не являются техническим пропуском и должны быть устранены до соответствующего release gate.

Каждый начатый release train должен доходить до официального immutable Public Stable. Stable tag/assets и исторические release identities не переписываются задним числом.

## 5. Stable и сайт

После нового Public Stable Control Center exact release identity передаётся web-потоку. Public site обязан проверить и при необходимости обновить:

- текущую версию;
- downloads/artifacts;
- install/update инструкции;
- release notes;
- capability/status claims.

Development capability запрещено описывать как доступную пользователю до собственной qualification и Stable publication.

## 6. UI/UX, Figma и локализация

Product Web UI должен быть современным, визуально цельным, отполированным, простым и понятным и соответствовать актуальным UI/UX best practices.

Обязательны:

- единая design system;
- Figma как рабочий design-source для design system, макетов и visual QA;
- responsive desktop/tablet/mobile;
- типографика, grid/spacing, единая система компонентов, форм, таблиц, графиков и иконок;
- keyboard navigation и accessibility;
- явные loading/empty/error/unknown/stale/degraded состояния;
- status не кодируется только цветом;
- русский и английский пользовательские языки через единый i18n-слой;
- hardcoded RU/EN user-facing строки в новой/изменяемой реализации не считаются завершённой работой.

Для 0.43 основной visual/UX pass выполняется в Slice I вместе с Product Web UI. Slice J включает обязательный Visual/UX Quality Gate. UI не должен опережать реальные contracts/API и создавать ложное впечатление уже доступной backend capability.

## 7. Документация

Каждый существенный change до начала работы сверяет актуальные нормативные документы, а после изменения architecture/API/UI/security/recovery/release state проверяет необходимость обновления:

- канонического Google Drive RoadMap/ТЗ;
- `ROADMAP.md`, `ARCHITECTURE.md`, `docs/REQUIREMENTS_RU.md` и профильных contracts/docs;
- release/Stable документации;
- production-facing site/LK claims.

Documentation Drift является дефектом процесса и release blocker для затронутой capability. Документация ведётся максимально на русском языке.
