# Grill Questions

Extends `rune` skill's question bank with project-bootstrap-specific blocks. Setup-session asks these in Phase 2 in 3–5 question batches via `AskUserQuestion`.

Tag legend:

- **[CRITICAL]** — block on missing answer; user must answer or explicitly defer with consequences acknowledged.
- **[HIGH]** — strongly recommended; "don't know" allowed → marks `confidence: MED`.
- **[MEDIUM]** — nice to have; skip if user uninterested.

---

## Block A — Project shape [CRITICAL]

A1. In one sentence: what does this project do?
A2. Who is the primary user? (developer, end-user, internal team, automated process)
A3. What does "success" look like in 3 months? (one or two concrete outcomes)
A4. Is this a service, library, CLI, app, batch job, or something else?
A5. Greenfield, port of an existing system, or replacement for a legacy one?

Branch:

- A4 = service → ask Block D (boundaries) first.
- A4 = library → skip Block F (ops & lifecycle) entirely; ask Block H (conventions) extra-carefully.
- A5 = port/replacement → ask "what artifact from the legacy system should we mirror or improve on?"

---

## Block B — Stack [CRITICAL]

B1. Primary language?
B2. Framework (if any) — be specific (Next.js App Router vs Pages, FastAPI vs Flask, gRPC vs REST)?
B3. Runtime / target — Node version, Python version, Go version, browser-only, serverless, container?
B4. Data store(s)? (Postgres, SQLite, Redis, S3, none)
B5. Deploy target — local-only, single VM, k8s, Vercel, AWS Lambda, mobile store, on-prem?
B6. Package manager preference (npm/pnpm/yarn, pip/poetry/uv, go modules)?

Branch:

- B4 = none → skip "schema migrations" later.
- B5 = local-only → skip Block F entirely.
- B5 = serverless → ask "cold-start budget? max execution time?" in Block C.

---

## Block C — Constraints [CRITICAL]

C1. Hard deadline or milestone? (date + what must exist by then)
C2. Privacy / compliance — does this handle PII, payments, health, regulated data?
C3. Performance ceilings — p95 latency, throughput, memory cap?
C4. Cost ceiling — infra budget per month?
C5. Reliability target — SLO if any, or "best effort"?

Branch:

- C2 = PII / regulated → trigger Block D auth question even if A4 ≠ service. Add a CLAUDE.md "Compliance" section.
- C3 named → seed a backlog task "Establish perf baseline + benchmark".

---

## Block D — Boundaries & integrations [HIGH]

D1. External systems consumed (APIs, queues, third-party SDKs)?
D2. External consumers (who calls us, in what shape)?
D3. Auth model — none, API keys, OAuth, session, mTLS, magic link, IDP?
D4. Secrets management — env vars, .env files, vault, KMS, 1Password CLI?
D5. Background jobs / scheduling — none, cron, queue (which)?

---

## Block E — Quality bar [HIGH]

E1. Test depth expectation — unit only, unit + integration, e2e, contract, fuzz?
E2. Coverage target — none, threshold %, or "meaningful tests only"?
E3. Review depth — solo merges OK, PR review required, multi-perspective-review required, security review when?
E4. Definition of done — for a typical PR, what gate must pass before merge?
E5. Linter / formatter preferences — eslint+prettier, ruff+black, gofmt+golangci-lint, biome, custom?

---

## Block F — Ops & lifecycle [MEDIUM] (skip if local-only)

F1. CI provider — GitHub Actions, Circle, none?
F2. Branch model — trunk-based, gitflow, feature branches + PR?
F3. Release cadence — continuous, weekly, ad-hoc?
F4. Monitoring — logs only, metrics, tracing, alerting (who pages)?
F5. On-call story — solo, rotation, none?

---

## Block G — Team & ownership [MEDIUM]

G1. Who else commits to this repo?
G2. Code review reviewers — fixed set, anyone, none?
G3. Decision-making — who has the final call on architecture vs feature priority?
G4. Communication — where do questions and decisions get logged (Slack, GitHub Discussions, decisions/ in repo)?

---

## Block H — Conventions [MEDIUM]

H1. Naming conventions — camelCase / snake_case / kebab-case for files, dirs, identifiers?
H2. Commit format — conventional commits, free-form, signed-off?
H3. PR title format?
H4. File header / banner — none, license SPDX, copyright?
H5. Docs location — README only, docs/ dir, ADRs in `decisions/`, none?

---

## Block I — Setup options [HIGH] (drives Phase 5 / 6 / 8)

I1. Enable hooks? (caveman-commit on commit, reminder on stop, etc.) — yes / no / which?
I2. Default Bash allowlist — strict (only explicit allow), permissive (allow common dev commands)?
I3. Want a project-specific orchestrator agent on top of `build-session`? (Default: no.) If yes, what responsibility does it cover that `build-session` cannot?
I4. Initial backlog size — small (3 tasks: unblock the unblockers), medium (5 tasks: include first feature), large (7 tasks: include CI + Docker + first feature)?
I5. First commit message — default `chore: initial project setup via /setup-session` OK, or override?

---

## Branching cheatsheet

| If user says... | Then skip / add |
|---|---|
| "library, no service" | Skip Block D, F. Add "publish to npm/PyPI/proxy.golang.org?" question. |
| "internal CLI only" | Skip Block D's external-consumer question. |
| "no DB" | Skip B4 follow-up + migration tasks in Phase 7. |
| "personal side project" | Drop Block G entirely. Defer Block F to "later". |
| "regulated / PII" | Treat all of Block C and D as [CRITICAL] regardless of default tag. Add a CLAUDE.md compliance section. |
| "I'll figure it out as I go" on a stack question | Refuse to proceed. State that the scaffold cannot be written without it. Ask the user to pick a default, with the option to change later. |

---

## Question delivery rules (enforced by SKILL.md Phase 2)

- Batches of 3–5 via `AskUserQuestion`. Never single questions except when a follow-up branches.
- If a question is already answered by intake, confirm in a single sentence — do not re-ask.
- Track which blocks are open vs closed. After each batch, print: `Blocks closed: {A, B}. Open: {C [CRITICAL], D, E}. Next batch: C + D.`
- On any [CRITICAL] question, if user declines twice in a row → Hard Stop 1.
