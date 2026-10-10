# Knowledge Base

Everything we design and decide lives here, next to the code.

| Folder / file | What goes in it |
|---|---|
| `HANDOFF.md` | Where the work stopped and what to do next — read first |
| `00-roadmap.md` | The full plan, phase by phase, and the $0 cost plan |
| `01-app-versions.md` | What the app can do after each phase |
| `Notes.md` | Concepts to revisit in free time — one short note per concept we discuss |
| `phases/` | One folder per part: design.md, plan.md, learnings.md, diagrams/ — plus the progress tracker |
| `concepts/` | One note per concept, written as we hit it (e.g. `load-balancing.md`) |
| `adr/` | Architecture Decision Records — one file per real decision (`0001-use-postgres.md`) |
| `diagrams/` | D2 sources (`.d2`) and their rendered `.svg` |

## Diagrams (D2, sketch mode)

```sh
./render.sh                                # render every diagram to SVG
./render.sh --watch diagrams/03-v1-architecture.d2   # live preview in the browser while editing
```

Docs embed the SVG: `![v1](diagrams/03-v1-architecture.svg)`.
Always commit both the `.d2` and the `.svg`.

D2 docs: https://d2lang.com/tour/intro — sketch mode: https://d2lang.com/tour/sketch

## How a concept note is written

1. **The problem** — what broke or what we need, in one paragraph
2. **Diagram**
3. **Options** — 2–3 ways to solve it, with trade-offs
4. **What we did** — link to the ADR and the code
5. **What we measured** — numbers from the load test
