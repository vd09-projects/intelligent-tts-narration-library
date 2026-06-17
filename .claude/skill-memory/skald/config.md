<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Skald — Project Config

## skill_registry

Project-installed producer skills. `multi-perspective-review` defaults retained from skald's built-ins.

```yaml
skill_registry:
  mimir:
    producer_role: planner
    plan_types:
      - architecture
      - task
    default_consumer_role:
      architecture: none           # no domain-expert skill installed
      task: implementation
  sindri:
    producer_role: implementation
    plan_types:
      - build
    default_consumer_role:
      build: review
  multi-perspective-review:
    producer_role: review
    plan_types:
      - findings
    default_consumer_role:
      findings: implementation
```

## default_owner

```
default_owner: vd
```

## slug_style

```
slug_style: kebab
slug_prefix: ""
```

## confirm_existing_match_threshold

```
confirm_existing_match_threshold: 0.7
```

## index_format

```
index_format: markdown
```

## status_overrides

```
status_overrides: {}
```

## Notes

- Solo project — `default_owner: vd` matches user handle.
- No domain-expert skill installed → `architecture` artifacts route with `consumer_role: none`. User reads and routes manually.

confidence: HIGH
