package presets

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
)

// Preset defines a complete tool configuration with sensible defaults
type Preset struct {
	Name        string   `yaml:"name" toml:"name"`
	Phase       string   `yaml:"phase" toml:"phase"`
	Image       string   `yaml:"image" toml:"image"`
	Description string   `yaml:"description,omitempty" toml:"description,omitempty"` // Usage notes and constraints, shown by `preset info`
	Hardened    bool     `yaml:"hardened,omitempty" toml:"hardened,omitempty"`       // Uses Docker Hardened Image (dhi.io)
	Command     string   `yaml:"command" toml:"command"`
	Entrypoint  []string `yaml:"entrypoint" toml:"entrypoint"`
	// ImageEntrypoint records the ENTRYPOINT the pinned image itself declares —
	// not an override, a fact about the image. An empty slice means the image
	// declares none; absent means nobody has recorded it yet.
	//
	// It exists so the rule of #338 can be checked offline. A command that
	// repeats its image's entrypoint hands the tool its own name as an
	// argument, and three presets shipped that way for as long as they existed
	// — goreleaser, docker-buildx and commitlint (#278, #336) — because
	// local_behavior short-circuits before the command reaches a container, so
	// the presets most protected from doing damage were the least likely to be
	// found broken.
	//
	// Derived data in a catalogue that avoids it, and justified the same way the
	// pinned digest is: an entrypoint is a property *of that digest*, so it can
	// only change when the digest does — through a promotion, the one gesture
	// that already rewrites the image line.
	ImageEntrypoint []string          `yaml:"image_entrypoint,omitempty" toml:"image_entrypoint,omitempty"`
	Workdir         string            `yaml:"workdir" toml:"workdir"`
	Volumes         []string          `yaml:"volumes" toml:"volumes"`
	Env             map[string]string `yaml:"env" toml:"env"`
	ConfigFiles     []string          `yaml:"config_files" toml:"config_files"`
	Options         map[string]Option `yaml:"options" toml:"options"`
	RequireCI       bool              `yaml:"require_ci" toml:"require_ci"`                       // Requires CI environment
	LocalBehavior   string            `yaml:"local_behavior" toml:"local_behavior"`               // draft, no-push, dry-run, disabled
	Privileged      bool              `yaml:"privileged,omitempty" toml:"privileged,omitempty"`   // Requires root privileges (skip user mapping)
	Ephemeral       bool              `yaml:"ephemeral,omitempty" toml:"ephemeral,omitempty"`     // Never reuse: this container writes to disk (#434)
	PullPolicy      string            `yaml:"pull_policy,omitempty" toml:"pull_policy,omitempty"` // always, if-not-present, never (default: env-based)
	Timeout         string            `yaml:"timeout,omitempty" toml:"timeout,omitempty"`         // duration string (e.g., "5m", "45m"), default: 30m
	DNS             []string          `yaml:"dns,omitempty" toml:"dns,omitempty"`                 // DNS servers (IP addresses); empty keeps the runtime default (#543)
}

// Option defines a configurable parameter for a preset.
//
// There is deliberately no `default` field (#299). MergeWith only visits the
// keys present in the user's overrides, so a declared default never reached a
// container: 45 of them shipped, none applied. Applying them instead was the
// worse option — `trivy` declared `--exit-code 0`, which never fails a scan,
// and `bandit` declared values for `-ll`/`-i`, which are counters and reject
// one. A default nobody has ever run is a default nobody has ever validated.
//
// A preset that genuinely needs a value puts it in its `command` (or `env`),
// where every run exercises it.
type Option struct {
	Type        string `yaml:"type" toml:"type"`                 // string, bool, int, array
	Description string `yaml:"description" toml:"description"`   // Help text
	EnvVar      string `yaml:"env_var" toml:"env_var"`           // Maps to environment variable
	CommandFlag string `yaml:"command_flag" toml:"command_flag"` // Maps to command flag
}

// MergeWith merges user overrides into the preset
func (p *Preset) MergeWith(overrides map[string]any) *Preset {
	merged := *p

	if image, ok := overrides["image"].(string); ok {
		merged.Image = image
	}
	if command, ok := overrides["command"].(string); ok {
		merged.Command = command
	}
	// Entrypoint and volumes arrive as []any when decoded from TOML through
	// a map[string]any pass (which is how cidx.toml [containers.X] sections
	// reach this function). The previous []string type assertion silently
	// dropped them — see #143. Accept both shapes via normalizeStringSlice.
	if entrypointRaw, hasEntrypoint := overrides["entrypoint"]; hasEntrypoint {
		if entrypoint, ok := normalizeStringSliceOverride(entrypointRaw); ok {
			merged.Entrypoint = entrypoint
		}
	}
	if workdir, ok := overrides["workdir"].(string); ok {
		merged.Workdir = workdir
	}
	if volumesRaw, hasVolumes := overrides["volumes"]; hasVolumes {
		if volumes, ok := normalizeStringSliceOverride(volumesRaw); ok {
			merged.Volumes = volumes
		}
	}
	// Merge per-key env overrides from cidx.toml.
	// TOML decoding through map[string]any yields map[string]any for nested
	// inline tables (e.g. `env = { FOO = "bar" }`), so we must accept both
	// shapes. Per-key, the user's value overrides the preset's on collision;
	// preset keys not mentioned by the user are preserved (closes #123).
	if envRaw, hasEnv := overrides["env"]; hasEnv {
		userEnv, ok := normalizeEnvOverride(envRaw)
		if ok {
			// Copy preset env first so we don't mutate the original map
			// (Preset is shallow-copied above; the Env map is shared).
			newEnv := make(map[string]string, len(merged.Env)+len(userEnv))
			for k, v := range merged.Env {
				newEnv[k] = v
			}
			for k, v := range userEnv {
				newEnv[k] = v
			}
			merged.Env = newEnv
		}
	}

	if pullPolicy, ok := overrides["pull_policy"].(string); ok {
		merged.PullPolicy = pullPolicy
	}
	if timeout, ok := overrides["timeout"].(string); ok {
		merged.Timeout = timeout
	}
	if dnsRaw, has := overrides["dns"]; has {
		if dns, ok := normalizeStringSliceOverride(dnsRaw); ok {
			merged.DNS = dns
		}
	}

	// Merge options with preset options
	for optName, optValue := range overrides {
		if opt, exists := merged.Options[optName]; exists {
			merged = applyOption(&merged, optName, opt, optValue)
		}
	}

	return &merged
}

// applyOption applies a specific option value to the preset
func applyOption(preset *Preset, name string, opt Option, value any) Preset {
	p := *preset

	// A boolean option is a switch, not a key/value pair. Appending its value
	// produced "--fix true" — a stray argument most tools reject (#214).
	//
	// The declared type is also the only type check the merge performs, and
	// applyOption cannot report an error (MergeWith has no error return). A
	// non-boolean value here is a typo that would otherwise surface as an
	// opaque container failure, so we name it and keep the preset default.
	enabled := true
	if opt.Type == "bool" {
		parsed, ok := toBool(value)
		if !ok {
			log.Printf("warning: option %q of preset %q expects a boolean (got %v) — override ignored", name, p.Name, value)
			return p
		}
		enabled = parsed
	}

	if opt.Type == cargoHomeType {
		return withCargoSeed(p, toString(value))
	}

	// Apply to environment variable if specified
	if opt.EnvVar != "" {
		if p.Env == nil {
			p.Env = make(map[string]string)
		}
		p.Env[opt.EnvVar] = toString(value)
	}

	// Apply to command flag if specified. A true boolean emits the flag alone;
	// a false one emits nothing at all.
	if opt.CommandFlag != "" && enabled {
		flagValue := toString(value)
		if opt.Type == "bool" {
			flagValue = ""
		}
		p.Command = appendCommandFlag(p.Command, opt.CommandFlag, flagValue)
	}

	return p
}

// cargoHomeType is the option type that points a Rust preset at a prepared
// cargo home (#553).
const cargoHomeType = "cargo_home"

// cargoSeedMount is where a prepared cargo home appears inside the container.
const cargoSeedMount = "/cidx/cargo-seed"

// withCargoSeed lets a Rust preset run offline from a prepared cargo home.
//
// The directory is mounted read-only, so no execution writes into a cache
// another one reads. Cargo cannot work from a read-only home though -- it locks
// files in the registry even when nothing is fetched -- so the container starts
// by copying the seed into a CARGO_HOME of its own, and every write lands in
// that copy. Which is what makes the result per execution: containers are named
// from the workspace, and a run has its own checkout.
func withCargoSeed(p Preset, dir string) Preset {
	p.Volumes = append(append([]string(nil), p.Volumes...), dir+":"+cargoSeedMount+":ro")
	env := make(map[string]string, len(p.Env)+2)
	for k, v := range p.Env {
		env[k] = v
	}
	env["CARGO_HOME"] = "/tmp/cargo-home"
	env["CARGO_NET_OFFLINE"] = "true"
	p.Env = env

	const seed = `mkdir -p "$CARGO_HOME" && cp -a ` + cargoSeedMount + `/. "$CARGO_HOME"/ && `
	if rest, ok := strings.CutPrefix(p.Command, shellCommandPrefix); ok && len(rest) >= 2 {
		if quote := rest[0]; (quote == '\'' || quote == '"') && rest[len(rest)-1] == quote {
			p.Command = shellCommandPrefix + string(quote) + seed + rest[1:]
			return p
		}
	}
	p.Command = shellCommandPrefix + "'" + seed + p.Command + "'"
	return p
}

// shellCommandPrefix marks a command that wraps a script for a container shell.
const shellCommandPrefix = "sh -c "

// appendCommandFlag returns command with " <flag> <value>" added, or just
// " <flag>" when value is empty — a valueless flag is a switch (#214).
//
// For a shell-wrapped command (`sh -c '<script>'`) the flag must land *inside*
// the quotes. Appended after the closing quote it becomes an argument of sh
// instead of the wrapped tool, and executor.parseCommand — which unquotes the
// script by stripping matching first/last quote characters — then fails to
// unquote at all and hands the whole `'<script>' --flag value` string to sh as
// a single command name (exit 127). See #200.
//
// The quote pair recognised here is exactly the one executor.parseCommand
// strips, so composition and execution agree by construction. Anything that
// does not match that shape (plain commands, unbalanced quoting) keeps the
// historical append-at-the-end behaviour.
func appendCommandFlag(command, flag, value string) string {
	suffix := " " + flag
	if value != "" {
		suffix += " " + value
	}

	if rest, ok := strings.CutPrefix(command, shellCommandPrefix); ok && len(rest) >= 2 {
		quote := rest[0]
		if (quote == '\'' || quote == '"') && rest[len(rest)-1] == quote {
			return command[:len(command)-1] + suffix + string(quote)
		}
	}

	return command + suffix
}

// normalizeStringSliceOverride coerces a TOML-decoded array value into []string.
// TOML decoding through map[string]any yields []any for arrays, not []string,
// so we accept both shapes. Non-string elements are skipped silently; if any
// element is unconvertible the whole slice is rejected (returns false) so the
// caller can preserve the preset's default rather than emit a half-built list.
func normalizeStringSliceOverride(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return s, true
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	default:
		return nil, false
	}
}

// PresetFromOverrides constructs a Preset from a custom container declaration
// in cidx.toml. A declaration is a `[containers.NAME]` section that has an
// `image` field present — that signals a brand-new container, not an override
// of a known preset. The returned Preset is filled from the overrides map;
// fields absent from the map keep their zero value.
//
// This implements the user-facing contract documented in
// examples/cidx-complete.toml (custom containers section) and closes #142.
func PresetFromOverrides(name string, overrides map[string]any) Preset {
	p := Preset{Name: name}

	if phase, ok := overrides["phase"].(string); ok {
		p.Phase = phase
	}
	if image, ok := overrides["image"].(string); ok {
		p.Image = image
	}
	if command, ok := overrides["command"].(string); ok {
		p.Command = command
	}
	if entrypointRaw, ok := overrides["entrypoint"]; ok {
		if entrypoint, ok := normalizeStringSliceOverride(entrypointRaw); ok {
			p.Entrypoint = entrypoint
		}
	}
	if workdir, ok := overrides["workdir"].(string); ok {
		p.Workdir = workdir
	}
	if volumesRaw, ok := overrides["volumes"]; ok {
		if volumes, ok := normalizeStringSliceOverride(volumesRaw); ok {
			p.Volumes = volumes
		}
	}
	if envRaw, ok := overrides["env"]; ok {
		if env, ok := normalizeEnvOverride(envRaw); ok {
			p.Env = env
		}
	}
	if privileged, ok := overrides["privileged"].(bool); ok {
		p.Privileged = privileged
	}
	if ephemeral, ok := overrides["ephemeral"].(bool); ok {
		p.Ephemeral = ephemeral
	}
	if pullPolicy, ok := overrides["pull_policy"].(string); ok {
		p.PullPolicy = pullPolicy
	}
	if timeout, ok := overrides["timeout"].(string); ok {
		p.Timeout = timeout
	}
	if dnsRaw, has := overrides["dns"]; has {
		if dns, ok := normalizeStringSliceOverride(dnsRaw); ok {
			p.DNS = dns
		}
	}

	return p
}

// Lookup resolves the preset a `[containers.NAME]` section runs, and says what
// is left of the section to merge onto it.
//
// Three shapes, tried in this order:
//   - an alias, `preset = "prettier"`: the catalogue preset it names, run under
//     the section's own name (so it gets a container of its own) with the
//     section's overrides on top. The image stays declared once, in the
//     catalogue (#292);
//   - a catalogue preset by name, the section being its overrides;
//   - a custom container, a section with an `image` and no catalogue preset.
//
// The name an alias takes must not be a catalogue preset's: `[containers.trivy]
// preset = "gitleaks"` would run one tool under another's name.
func Lookup(name string, overrides map[string]any) (Preset, map[string]any, error) {
	if base, ok := overrides["preset"].(string); ok && base != "" {
		if Exists(name) {
			return Preset{}, nil, fmt.Errorf("[containers.%s] sets preset = %q, but %q is itself a catalogue preset: an alias needs a name of its own", name, base, name)
		}
		preset, err := Get(base)
		if err != nil {
			return Preset{}, nil, fmt.Errorf("[containers.%s] sets preset = %q, which is not a catalogue preset (an alias names a preset, not another alias)", name, base)
		}
		preset.Name = name
		return preset, overrides, nil
	}
	if preset, err := Get(name); err == nil {
		return preset, overrides, nil
	}
	if !IsCustomDeclaration(overrides) {
		return Preset{}, nil, fmt.Errorf("container %q is not a built-in preset and has no [containers.%s] declaration with an `image` field", name, name)
	}
	// A custom container is its declaration in full: nothing is left to merge.
	return PresetFromOverrides(name, overrides), nil, nil
}

// DNSProblem reports why the `dns` key of a `[containers.NAME]` section cannot
// be used, or nil. Docker takes IP addresses only: a hostname here would fail at
// container creation, after the pull, so it is refused when the file is read.
func DNSProblem(name string, overrides map[string]any) error {
	raw, has := overrides["dns"]
	if !has {
		return nil
	}
	servers, ok := normalizeStringSliceOverride(raw)
	if !ok {
		return fmt.Errorf("[containers.%s] dns must be a list of IP addresses, e.g. dns = [\"1.1.1.1\"]", name)
	}
	for _, server := range servers {
		if net.ParseIP(server) == nil {
			return fmt.Errorf("[containers.%s] dns: %q is not an IP address", name, server)
		}
	}
	return nil
}

// IsCustomDeclaration reports whether the given override section declares a
// brand-new container (image field present) rather than overriding an
// existing preset. Used by validator and runner to decide between
// preset-override semantics and custom-container semantics.
func IsCustomDeclaration(overrides map[string]any) bool {
	if overrides == nil {
		return false
	}
	image, ok := overrides["image"].(string)
	return ok && image != ""
}

// normalizeEnvOverride coerces an env override value into map[string]string.
// Accepts the in-process map[string]string form and the TOML-decoded
// map[string]any form (where each value is itself converted via toString).
// Returns (nil, false) for any other shape so callers can ignore it safely.
func normalizeEnvOverride(v any) (map[string]string, bool) {
	switch env := v.(type) {
	case map[string]string:
		return env, true
	case map[string]any:
		out := make(map[string]string, len(env))
		for k, val := range env {
			// Only coerce scalar values; reject nested tables / arrays so a
			// malformed user config surfaces upstream rather than producing
			// "<nil>" strings silently.
			switch val.(type) {
			case string, int, int64, bool, float64:
				out[k] = toString(val)
			default:
				return nil, false
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// toBool coerces an option value into a boolean. Both shapes an override can
// take are accepted: the native TOML boolean (`fix = true`) and the string
// form, which is what a quoted value (`fix = "true"`) or an expanded
// `${VAR}` placeholder yields by the time it reaches MergeWith. Anything
// strconv.ParseBool rejects ("yes", "on", "1.0", a typo) returns false so the
// caller can refuse the override rather than guess.
func toBool(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(b))
		return parsed, err == nil
	default:
		return false, false
	}
}

// toString converts interface{} to string
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}
