# House style

Hive prepends a `HOUSE STYLE` section immediately below the agent kick header,
before the policy body. It applies to every backend and every template source:
config templates, GitHub-sourced prompts, embedded policies and hardcoded
fallbacks, for scheduled and manual scheduler-generated kicks.

## Sources and precedence

The first nonblank, accepted source wins:

1. `HIVE_STYLE.md` at the checkout root of the agent's primary repository.
   The root uses `project.checkouts_dir`, then `policies.local_dir` when
   `policies.repo` matches, just like [AGENTS.md](agents-md.md). Scoped agents
   use their own primary repo. When no checkout is wired, Hive fetches the file
   via the GitHub Contents API on that repo's default branch. A wired checkout
   with no file falls through to the hive default, without fetching remotely.
   Non-GitHub forges use the local checkout or the hive default.
2. `project.style_file`, a path on the hive host.
3. `project.style`, inline Markdown in `hive.yaml`.
4. The built-in rules: smallest complete diff, no speculative abstractions,
   follow existing patterns, tests for behaviour changes, and DCO sign-off.

The source is shown as `repo`, `hive` or `built-in` in the section header,
the dashboard agent card and kick-history listing/detail. Historical source
metadata describes the delivered kick, even if the operator later edits config.
Older kicks without the metadata have no style source recorded.

```yaml
project:
  org: your-org
  repos: [your-repo]
  primary_repo: your-repo
  style_file: /data/hive-style.md
  style: |
    Keep diffs focused. Follow existing patterns.
    Add tests for changed behaviour and sign off every commit.
```

A repo can override that default with a root `HIVE_STYLE.md`, for example:

```markdown
Keep changes small and complete. Reuse the package's existing helpers.
Test changed behaviour. Sign off commits with git commit -s.
```

## Limits and input defense

Every source has a 4096-byte (4 KiB) cap, measured before whitespace trimming.
Oversized text is rejected whole and resolution continues to the next source;
it is never truncated into partial instructions. Missing, unreadable or blank
files and failed remote fetches also fall through. Reads happen at kick time,
so edits apply to the next kick without rebuilding policies.

Repo-supplied text, whether local or fetched, is untrusted input. It passes
through `pkg/ioscan` using the untrusted `issue_body` threshold and existing
classifier settings. A blocking verdict refuses the entire repo style, emits
an `ioscan_block` event plus a `style_refused` audit entry naming the repo and
file, and tries the hive default. Refused instructions are not copied into
the refusal detail. Existing `ioscan.enabled` and `ioscan.block_at` settings
apply. Host-local and inline hive defaults are operator-authored.

`AGENTS.md` processing is unchanged. There are no per-agent style overrides
and no new telemetry. Configuration keys are also listed in the
[operator reference](operator-reference.md).
