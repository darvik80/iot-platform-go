# Diagram Style Guide

All Mermaid diagrams in `docs/` follow these rules.
The **Platform Architecture** diagram in `diagrams.md` is the canonical reference.

**Agent rule**: `.cursor/rules/diagrams.md` — Cursor/Qoder agents read this
automatically when editing files in `docs/`.

---

## 1. Diagram Types

| Type | Use for |
|------|---------|
| `graph TB` / `flowchart TB` | Architecture, component relationships |
| `sequenceDiagram` | Time-ordered interactions between actors |

All architecture graphs use **top-to-bottom** direction (`TB`).

---

## 2. Dark Theme (graph / flowchart)

Every `graph` or `flowchart` diagram must define explicit `style` directives.

### Color Palette

| Role | fill | stroke |
|------|------|--------|
| Platform outer | `#1a1a2e` | `#16213e` |
| Platform inner layers | `#0f3460`, `#1a1a2e`, `#16213e` | `#16213e`, `#0f3460`, `#1a1a2e` |
| Devices | `#0f3460` | `#16213e` |
| External / secondary | `#16213e` | `#1a1a2e` |
| Text | `color:#eee` | — |

### Style Block Template

```
style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
style Transport fill:#0f3460,stroke:#16213e,color:#eee
style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
style Services fill:#16213e,stroke:#1a1a2e,color:#eee
style Devices fill:#0f3460,stroke:#16213e,color:#eee
style External fill:#16213e,stroke:#1a1a2e,color:#eee
```

### Hierarchical Layout

Use **nested subgraphs** inside the Platform container to enforce strict
layer ordering. Each nested subgraph gets a descriptive label and a distinct
color from the palette. Invisible layers (`fill:none,stroke:none`) are **not
allowed** — every layer must be visually distinguishable.

```
subgraph Platform["Cogitor IoT Platform (single Go binary)"]
    subgraph Transport["MQTT Transport"]
        ...
    end
    subgraph Core["Core Processing"]
        ...
    end
    subgraph Services["Services & Storage"]
        ...
    end
end
```

---

## 3. Node Labels

Every node must include **technical details** in parentheses after the name:

```
Broker["MQTT Broker<br/>(mochi-mqtt, embedded)"]
DB["SQLite<br/>(WAL mode)"]
API["REST API<br/>(chi router)"]
```

Use `<br/>` for line breaks inside labels.

---

## 4. Edge Comments (graph / flowchart)

Group related edges and separate groups with `%%` comment lines describing
the data flow direction:

```
%% Device → Transport
ESP1 & ESP2 & ESP3 -->|"MQTT TCP :1883"| Broker

%% Transport → Core
Broker --> ACL
ACL --> Ingest

%% Core internal
Ingest --> PluginHub

%% Core → Services
Ingest --> DB

%% Services ↔ External
Rules -->|"platform/v1/events/..."| NR
```

---

## 5. Sequence Diagrams

### Note Separators

Use `Note over` blocks for **major lifecycle phases** only:
registration, binding, device connect/disconnect, offline detection.
Do NOT add notes for every message — short flows stay clean.

Good (lifecycle diagram with phases):
```
Note over Pl: Старт процесса
Pl->>P: POST /register
P-->>Pl: {ok: true, token: "..."}

loop каждые 30 сек
    Pl->>P: platform.heartbeat
    P-->>Pl: {ok: true}
end

Note over U: Привязка плагина к устройству
```

Bad (short flow — notes add noise):
```
Note over D: Телеметрия
D->>P: MQTT telemetry
Note over P: Диспетчеризация
P->>Pl: plugin.on_event
```

### Participant Names

Use short aliases with descriptive display names:

```
participant Pl as Плагин
participant P as Платформа
participant D as Устройство
```

---

## 6. Centralization

All diagrams are stored in `docs/14-diagrams.md`. Source documents
(`01-prd.md`, `08-external-plugins.md`, `07-internal-plugins.md`) contain:

1. A link reference: `> **Diagram**: [Name](14-diagrams.md#anchor)`
2. The full inline mermaid block (for rendering in markdown previews)

Both copies must stay **in sync**. When updating a diagram, update it in
**both** `diagrams.md` and the source document.

---

## 7. Checklist

Before committing a new or updated diagram, verify:

- [ ] Dark theme `style` directives present (graph/flowchart only)
- [ ] Nested subgraphs with labels and distinct colors for platform layers
- [ ] Node labels include technical details in parentheses
- [ ] Edge groups separated by `%%` comment lines
- [ ] Sequence diagrams use `Note over` for major lifecycle phases only
- [ ] Diagram exists in both `diagrams.md` and the source document
- [ ] Both copies are identical
