---
description: Mermaid diagram style conventions for docs/
globs: docs/**/*.md
alwaysApply: true
---

# Mermaid Diagram Style

All Mermaid diagrams in `docs/` must follow these rules.
The **Platform Architecture** diagram in `docs/diagrams.md` is the canonical reference — copy its style exactly.

Full human-readable guide: `docs/diagram-style.md`.

## graph / flowchart

**Direction:** `graph TB` or `flowchart TB` (top-to-bottom).

**Dark theme — mandatory.** Every `graph`/`flowchart` must end with explicit `style` directives:

```
style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
style Transport fill:#0f3460,stroke:#16213e,color:#eee
style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
style Services fill:#16213e,stroke:#1a1a2e,color:#eee
style Devices fill:#0f3460,stroke:#16213e,color:#eee
style External fill:#16213e,stroke:#1a1a2e,color:#eee
```

Color palette: `#1a1a2e` / `#0f3460` / `#16213e` for fills, `color:#eee` for text.
Never use `fill:none,stroke:none` — every subgraph must be visually distinguishable.

**Nested subgraphs for hierarchy.** Inside the Platform container, use named subgraphs
with distinct colors to enforce strict layer ordering:

```
subgraph Platform["Cogitor IoT Platform (single Go binary)"]
    subgraph Transport["MQTT Transport"] ... end
    subgraph Core["Core Processing"] ... end
    subgraph Services["Services & Storage"] ... end
end
```

**Node labels** must include technical details in parentheses:

```
Broker["MQTT Broker<br/>(mochi-mqtt, embedded)"]
DB["SQLite<br/>(WAL mode)"]
```

**Edge comments** — group related edges, separate groups with `%%` comments:

```
%% Device → Transport
ESP1 & ESP2 & ESP3 -->|"MQTT TCP :1883"| Broker

%% Transport → Core
Broker --> ACL
ACL --> Ingest
```

## sequenceDiagram

**Participant names** — short aliases with descriptive display names:

```
participant Pl as Плагин
participant P as Платформа
participant D as Устройство
```

**Note separators** — use `Note over` for major lifecycle phases
(registration, binding, disconnect, offline detection). Do NOT add notes
for every single message — only for phase transitions.

## Centralization

All diagrams live in `docs/diagrams.md`. Source documents (`prd.md`,
`plugin-prd.md`, `plugins.md`) contain:

1. Link: `> **Diagram**: [Name](diagrams.md#anchor)`
2. Full inline mermaid block (identical copy)

**When updating a diagram, update BOTH copies.**

## Checklist

Before committing:

- [ ] `style` directives present (graph/flowchart)
- [ ] Nested subgraphs with labels + distinct colors (platform diagrams)
- [ ] Node labels have technical details in parentheses
- [ ] Edge groups separated by `%%` comments
- [ ] Diagram in both `diagrams.md` and source doc, identical
