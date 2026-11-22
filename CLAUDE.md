# Nithro

Security-first Discord bot by Nithron. Current phase: planning. Decisions: docs/decisions.md.

- Do not write implementation code until the PM has approved the plan.
- Stack: Go bot (disgo) + Go API; dashboard is a Vite + React + TypeScript SPA
  embedded in the Go API binary (see docs/decisions.md).
- Commits: author every commit as the repo owner (the configured git user) and add no
  `Co-Authored-By` or other Claude attribution lines to commits or PR descriptions.
- License: `LICENSE` must stay byte-identical with NithronOS and NithronAI. A change here
  means the same change there.
- Brand: logo = concept A (`<N>` brackets). Source SVGs live in `assets/brand/`; the
  rendered PNGs are generated from them by `assets/brand/build.sh`.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- When graphify-out/graph.json exists, run `graphify query "<question>"` before your first grep, search or source-file read for a codebase question, and put this rule in every subagent prompt that explores code. Read raw files only for lines the graph or an earlier read already located. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. When the answer is under a Markdown heading, `graphify section "<heading>"` prints that section's text, so the .md need not be opened. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
- A committed graphify-out/graph.json is rewritten whole on every build, so a large diff to it in a pull request is expected. Do not flag it as churn or suggest skipping the graph rebuild or its hooks.
