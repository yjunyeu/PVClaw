# PVClaw

PVClaw is a user-friendly control layer for creating purpose-specific,
privacy-preserving OpenClaw agents.

Its profile is the authoritative source of truth. OpenClaw and DefenseClaw
configuration will eventually be generated downstream from a PVClaw profile;
users should not need to maintain three independent configuration systems.

## Current milestone

PVClaw validates Profile V1 YAML, compiles it into in-memory desired state,
and produces a deterministic plan. It can now scaffold a PVClaw-managed agent
locally and reconcile that agent's OpenClaw configuration. DefenseClaw is not
modified in this milestone.

The local profile is authoritative. For agent `medical`, PVClaw owns only:

```text
<PVCLAW_HOME>/agents/medical/profile.yaml  # authoritative privacy intent
<PVCLAW_HOME>/agents/medical/data/         # persistent user-owned sandbox data
<PVCLAW_HOME>/agents/medical/state.json    # derived apply metadata, never input config
```

Reconciliation uses supported OpenClaw interfaces only:

- `openclaw agents add <agent-id> --workspace <path> --non-interactive` when
  the named agent is absent;
- `openclaw gateway call config.get --json` followed by `config.patch` with
  the returned `baseHash`, scoped to `agents.entries.<agent-id>`;
- `openclaw sandbox recreate --agent <agent-id> --force` only for existing
  agent sandbox/network/bind changes; and
- `openclaw sandbox explain --agent <agent-id> --json` after a successful
  reconciliation.

PVClaw does not rewrite `~/.openclaw/openclaw.json`, replace the OpenClaw
agent roster, or change unrelated global or agent configuration. Applying a
profile requires a reachable, authenticated Gateway with permission to call
`config.get` and `config.patch`. It does not invoke Docker directly.

The included examples are:

- `profiles/medical.example.yaml`: read-only data, no network, shell, browser,
  or web search.
- `profiles/work.example.yaml`: read-write data with network, shell, browser,
  and web search enabled.

## Usage

```bash
go run ./cmd/pvclaw validate profiles/medical.example.yaml
go run ./cmd/pvclaw plan --home /tmp/pvclaw-test profiles/medical.example.yaml
go run ./cmd/pvclaw create medical --template medical --home /tmp/pvclaw-test
```

The installed binary is named `pvclaw`:

```bash
pvclaw validate profiles/medical.example.yaml
pvclaw plan --home /tmp/pvclaw-test profiles/medical.example.yaml
pvclaw create medical --template medical --home /tmp/pvclaw-test
pvclaw diff medical --home /tmp/pvclaw-test
pvclaw apply medical --home /tmp/pvclaw-test
# Use --yes only for non-interactive, explicitly authorized application.
pvclaw apply medical --yes --home /tmp/pvclaw-test
```

`--home` selects the PVClaw root used for deterministic path derivation. When
it is omitted, the CLI uses `PVCLAW_HOME`, then falls back to `~/.pvclaw`.

`create` is local-only: it creates a profile and `data/` directory but never
contacts OpenClaw. `diff` reads the local profile and OpenClaw's current
configuration. `apply` uses the same profile to create or patch just that
agent, verifies the sandbox, then writes `state.json` only after success.

## Development

```bash
gofmt -w $(find cmd internal -name '*.go')
go test ./...
```
