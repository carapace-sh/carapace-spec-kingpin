# AGENTS.md

Guide for AI agents working in this repository.

## What this is

A Go library that generates [carapace-spec](https://github.com/carapace-sh/carapace-spec) YAML from a [kingpin v2](https://github.com/alecthomas/kingpin) `*kingpin.Application`. Consumers (CLI apps built with kingpin) call `spec.Register(app)` to attach a hidden `_carapace spec` subcommand that prints the spec; or call `spec.Command(app)` directly to obtain a `command.Command` without registering anything.

The entire public API is one file: `spec.go`. There is no `cmd/`, no `internal/`, no subpackages.

## Commands

All commands run from the repo root. Module requires Go 1.24 (see `go.mod`).

- `go build -v ./...` — build
- `go test -v -coverprofile=profile.cov ./...` — test (CI invocation; tests live in `spec_test.go`)
- `go vet ./...` — vet
- `gofmt -d -s .` — format check (CI fails the build if this prints anything; use `gofmt -w -s .` to fix)
- `staticcheck ./...` — static analysis (CI installs `honnef.co/go/tools/cmd/staticcheck@latest` then runs it)

CI (`.github/workflows/go.yml`) runs: build → test → format check → goveralls coverage → staticcheck, in that order. Match that set locally before pushing.

## Architecture & data flow

```
*kingpin.Application
   │  app.Model() → kingpin.CmdModel  (kingpin's introspection model)
   ▼
scrape(*kingpin.CmdModel, root bool) command.Command   (recursive)
   │  per flag:   build command.Flag, cmd.AddFlag(f)
   │  per subcmd: recurse with root=false
   ▼
command.Command  (github.com/carapace-sh/carapace-spec/pkg/command)
   │  yaml.Marshal
   ▼
YAML printed to stdout
```

Two entry points in `spec.go`:

- `Register(app *kingpin.Application)` — installs a hidden `_carapace` command with a `spec` subcommand whose action marshals `Command(app)` to YAML and prints it. This is the integration hook for consuming apps.
- `Command(app *kingpin.Application) command.Command` — pure conversion, no registration. Builds a synthetic root `kingpin.CmdModel` from `app.Name`, `app.Help`, and `app.Model().FlagGroupModel`/`CmdGroupModel`, then calls `scrape(..., true)`.

`scrape` is the only recursive function. It walks kingpin's `CmdModel` tree and maps each node to a `command.Command`:

- **Flags** → `command.Flag` with `Longhand: "--" + flag.Name` (see gotcha below), `Value: !flag.IsBoolFlag()`, `Required: flag.Required`, `Hidden: flag.Hidden`, `Default: strings.Join(flag.Default, ",")`. Shorthand set from `flag.Short` as `"-" + string(flag.Short)`. The `Default` field (carapace-spec v1.8.0+) carries kingpin's `FlagModel.Default` (`[]string`) into the spec's extended flag notation; multiple defaults are comma-joined.
- **Bool flag negation** — for `IsBoolFlag()` flags, a second hidden `--no-<name>` flag is added (with `Shorthand: ""`, `Hidden: true`, `Default: ""` cleared so the negation doesn't inherit the bool's default).
- **Subcommands** → recursed with `root=false`; the special `_carapace` command is skipped during recursion (`if subcmd.Name != "_carapace"`).
- **Persistent flags** — `Persistent: root` marks all flags on the root command as persistent (so they propagate to subcommands in the spec), and all non-root flags as non-persistent. This is a coarse heuristic, not per-flag.

`cmd.AddFlag(f)` (defined in `carapace-spec/pkg/command/command.go`) routes the flag into `PersistentFlags` when `f.Persistent` is true, else into `Flags`. The map key is `Flag.format()` (see `carapace-spec/pkg/command/flag.go`), which is where the gotcha below bites.

## Conventions

- Package name is `spec` (not `carapacespec`, not `kingpinspec`). Import path is the module path `github.com/carapace-sh/carapace-spec-kingpin`; consumers alias it as they like.
- No test files existed originally. `spec_test.go` now covers the `Default` mapping, bool-negation default clearing, and the persistent-flag heuristic. Follow the existing table-driven style and `keys` helper when adding more. Note `go.sum` pulls `stretchr/testify` indirectly via carapace-spec, but it is **not** a direct dependency — the tests use only stdlib `testing`, so no new deps are needed.
- `// TODO groups` in `scrape` is aspirational — kingpin v2.4.0's introspection model (`FlagModel`, `CmdModel`) does not expose group information, so there is nothing to map yet. Groups exist internally in kingpin (`flagGroup` in `app.go`) but are not surfaced through `Model()`. Don't attempt to implement group support without first adding group fields to kingpin's model.
- Code style: gofmt-simple (`-s`), tab-indented, short error handling (`panic(err.Error())` for the marshal error in `Register`).
- **Extended flag notation** is emitted by carapace-spec's `FlagSet.MarshalYAML` automatically when a flag has `Nargs != 0` **or** `Default != ""`. `scrape` only sets `Default` (never `Nargs`), so in this repo the extended form is triggered solely by flags with a non-empty default. A flag with a default renders as a map (`{description: ..., default: ...}`) rather than a bare string. Don't try to emit the extended form manually — just populate `Flag.Default` and let `FlagSet.MarshalYAML` handle the serialization.

## Gotchas

1. **Double-dash bug in longhand flags.** `scrape` sets `f.Longhand = "--" + flag.Name`, but `command.Flag.format()` *also* prepends `--` to `Longhand`. Result: emitted YAML keys look like `----verbose` instead of `--verbose`, and `--no-` negations become `----no-verbose&`. Verify with `go run` against a sample app before trusting output. The correct fix is `f.Longhand = flag.Name` (store the bare name); `format()` adds the dashes. Same issue applies to the `--no-` branch. **Do not "fix" this without checking downstream consumers** — carapace-spec may parse it back symmetrically, but the YAML is visibly wrong and will surprise anyone reading it.

2. **`Command(app)` builds its own root model rather than reusing `app.Model()` directly.** It constructs a fresh `kingpin.CmdModel` with `Name: app.Name`, `Help: app.Help`, and only copies `FlagGroupModel` and `CmdGroupModel` from `app.Model()`. Other fields on the application-level model are intentionally dropped.

3. **`Register` is idempotent-ish but not re-entrant.** It calls `app.GetCommand("_carapace")` and only creates the hidden `_carapace` command if absent, but it always adds a new `spec` subcommand under it. Calling `Register` twice on the same app will create duplicate `spec` subcommands and kingpin will panic on parse. Call it once.

4. **The `_carapace` command is filtered only during `scrape` recursion**, not in `Register`. `Register` creates `_carapace` intentionally; `scrape` skips it when walking subcommands so it never appears in the generated spec. If you add new introspection paths, preserve this skip or the spec will leak the internal command.

5. **`Persistent: root` is a command-level heuristic, not a flag-level property.** Every flag on the root command becomes persistent; every flag on a subcommand is non-persistent. Kingpin's own per-flag persistence model is not consulted. If a consuming app relies on a subcommand flag being persistent, the spec will not reflect that.

6. **Multi-value defaults are comma-joined, and `Repeatable` is never set.** kingpin's `FlagModel.Default` is `[]string` (e.g. `.Default("a","b","c")`), but carapace-spec's `Flag.Default` is a single `string`. `scrape` joins them with `","`. This is lossy because `scrape` also never sets `command.Flag.Repeatable` (kingpin's `FlagModel` doesn't expose repeatability — it's encoded in the `Value` type, not the model), so carapace-spec registers the flag with `fs.String()` rather than `fs.StringSlice()`. The comma-joined default is thus stored as one literal string, not split back into a slice. pflag's `stringSliceValue.Set` *would* CSV-parse it, but that path isn't reached. Acceptable for typical single-token defaults (paths, numbers); problematic for multi-value defaults containing commas.

7. **CI uses `actions/checkout@v7` with shallow clone** for non-tag pushes and **deep clone** for tag pushes. GoReleaser is commented out in the workflow — releases are not currently automated through CI. Dependabot auto-merges dependency bumps via `.github/workflows/dependabot.yml` using `secrets.DEPENDABOT_TOKEN`.

8. **`go.mod` pins a pseudo-version of carapace-spec** (`v1.7.2-0.20260703181424-753e457ae985`) because v1.8.0 is not yet tagged. Once carapace-spec v1.8.0 is released, run `go get github.com/carapace-sh/carapace-spec@v1.8.0 && go mod tidy` to move to the tagged version so dependabot can manage it normally.

## Key dependencies

- `github.com/alecthomas/kingpin/v2` v2.4.0 — the CLI framework being introspected. `scrape` consumes `kingpin.CmdModel`, `kingpin.FlagModel`, `kingpin.CmdGroupModel`, `kingpin.FlagGroupModel`.
- `github.com/carapace-sh/carapace-spec` (pinned to a pre-v1.8.0 pseudo-version) — the target spec format. Types used: `command.Command`, `command.Flag` (incl. `Flag.Default` and `Flag.Nargs`), and `command.FlagSet` (a `map[string]Flag` keyed by `Flag.format()`). v1.8.0 adds the `Default` field and extended-notation `default` YAML key; `FlagSet.MarshalYAML` switches to the extended map form when `Nargs != 0 || Default != ""`. `FlagSet.UnmarshalYAML` accepts `default` as string/int/bool/nil.
- `gopkg.in/yaml.v3` v3.0.1 — marshalling. The `command.Command` struct tags drive YAML key names.

The `carapace-spec` source is likely in your module cache under `~/go/pkg/mod/github.com/carapace-sh/` or alongside this repo at `../carapace-spec` if you have the carapace-sh monorepo checked out. Reading `carapace-spec/pkg/command/flag.go` and `command.go` is essential before changing `scrape`, since the flag key format and `AddFlag` routing logic live there.
