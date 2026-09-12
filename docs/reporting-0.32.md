# Reporting foundation for 0.32

Status: development-only foundation for the 0.32 release line. This document does not claim Public Stable availability.

## Scope

The 0.32 reporting layer starts with two deliberately small primitives:

1. a side-effect-free numeric formula engine over an already-authorized row;
2. deterministic UTF-8 CSV materialization with bounded rows, columns and cell sizes.

The reporting package is not an authorization boundary. Callers must construct rows only after tenant/scope authorization and must not pass fields the current subject is not allowed to read. Report definitions must therefore be evaluated after RBAC and tenant isolation, never before them.

## Formula security model

Formulas support numeric literals, row-field references, parentheses and `+`, `-`, `*`, `/` only. They do not support functions, strings, imports, environment access, file/network access, reflection or dynamic calls. Missing and non-numeric fields, division by zero, non-finite values, excessive source length, excessive token count and excessive nesting all fail closed.

This restricted grammar is intentional. It provides the arithmetic needed for capacity and operational reports without creating an expression-language execution surface.

## CSV security model

CSV export treats string values as untrusted. Values that spreadsheet applications could interpret as formulas after leading whitespace are prefixed with an apostrophe. Typed numeric values remain numeric text, so legitimate negative metrics are not altered.

Exports are bounded before materialization:

- maximum row count;
- maximum column count;
- maximum UTF-8 cell size;
- duplicate or empty headers rejected;
- each column must be exactly one direct field or one compiled formula;
- unsupported composite values fail closed instead of being stringified implicitly.

These constraints reduce formula-injection risk, accidental data-shape expansion and memory abuse from malformed report definitions.

## Integration contract

A future HTTP/service integration inside the 0.32 boundary should follow this order:

1. authenticate subject and resolve tenant/scope;
2. authorize requested report and every source field;
3. query or assemble bounded rows;
4. compile configured formulas;
5. evaluate formulas and export CSV;
6. emit an audit event containing report identity, subject, scope, row count and outcome, but not raw report data.

Do not persist credentials, secrets or unrestricted raw payloads in report definitions or audit metadata. Commercial/entitlement checks must remain fail-closed and separate from RBAC; having an entitlement must never grant additional data permissions.

## Current non-goals

This foundation does not yet provide a public HTTP endpoint, scheduled reports, dashboards, arbitrary functions, cross-tenant aggregation, background execution or commercial entitlement activation. Those require their own review and acceptance inside the 0.32 release boundary.
