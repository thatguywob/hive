package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/hive/crux"
	"github.com/hivecommons/hive/pkg/config"
	"github.com/hivecommons/hive/pkg/promptsrc"
)

func writeStyle(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHouseStylePrecedenceAndCap(t *testing.T) {
	for _, tc := range []struct {
		name, repo, file, inline, want, source string
	}{
		{name: "built-in", want: builtInHouseStyle, source: "built-in"},
		{name: "inline", inline: "Inline rules", want: "Inline rules", source: "hive"},
		{name: "file", file: "Host rules", inline: "Inline rules", want: "Host rules", source: "hive"},
		{name: "repo", repo: "Repo rules", file: "Host rules", inline: "Inline rules", want: "Repo rules", source: "repo"},
		{name: "repo at cap", repo: strings.Repeat("x", maxHouseStyleBytes), want: strings.Repeat("x", maxHouseStyleBytes), source: "repo"},
		{name: "oversize repo", repo: strings.Repeat("x", maxHouseStyleBytes+1), file: "Host rules", want: "Host rules", source: "hive"},
		{name: "oversize host file", file: strings.Repeat("x", maxHouseStyleBytes+1), inline: "Inline rules", want: "Inline rules", source: "hive"},
		{name: "oversize inline", inline: strings.Repeat("x", maxHouseStyleBytes+1), want: builtInHouseStyle, source: "built-in"},
		{name: "whitespace", repo: " \n", file: "\n", inline: " ", want: builtInHouseStyle, source: "built-in"},
		{name: "cap before trimming", repo: strings.Repeat(" ", maxHouseStyleBytes) + "x", inline: "Inline rules", want: "Inline rules", source: "hive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := &config.Config{Project: config.ProjectConfig{Org: "org", PrimaryRepo: "repo", Repos: []string{"repo"}, CheckoutsDir: root, Style: tc.inline, StyleFile: filepath.Join(root, "host.md")}}
			writeStyle(t, filepath.Join(root, "repo", "HIVE_STYLE.md"), tc.repo)
			writeStyle(t, cfg.Project.StyleFile, tc.file)
			s := New(cfg, slog.Default())
			body, source := s.resolveHouseStyle("scanner")
			if body != tc.want || source != tc.source {
				t.Fatalf("got (%q, %q), want (%q, %q)", body, source, tc.want, tc.source)
			}
		})
	}
}

func TestHouseStyleRefusalAudit(t *testing.T) {
	root := t.TempDir()
	writeStyle(t, filepath.Join(root, "repo", "HIVE_STYLE.md"), mediumInjectionTitle)
	cfg := &config.Config{Project: config.ProjectConfig{Org: "org", PrimaryRepo: "repo", Repos: []string{"repo"}, CheckoutsDir: root, Style: "Safe host rules"}}
	s := New(cfg, slog.Default())
	var actions []string
	s.SetAuditFunc(func(action, detail, agent string) {
		actions = append(actions, action)
		if strings.Contains(detail, mediumInjectionTitle) {
			t.Fatal("audit leaked refused instructions")
		}
		if action == "style_refused" && (!strings.Contains(detail, "repo=org/repo") || agent != "scanner") {
			t.Fatalf("missing refusal attribution: %s %s", detail, agent)
		}
	})
	body, source := s.resolveHouseStyle("scanner")
	if body != "Safe host rules" || source != "hive" {
		t.Fatalf("refused repo did not fall back: %q %s", body, source)
	}
	if strings.Join(actions, ",") != "ioscan_block,style_refused" {
		t.Fatalf("audit actions: %v", actions)
	}
	// Use the configured issue-body threshold, not an unrelated surface's policy.
	cfg.Ioscan.BlockAt = map[string]string{"issue_body": "high"}
	body, source = s.resolveHouseStyle("scanner")
	if body != mediumInjectionTitle || source != "repo" {
		t.Fatalf("issue-body threshold ignored: %q %s", body, source)
	}
}

type styleFetchFunc func(context.Context, string, string, string, string) (string, error)

func (f styleFetchFunc) GetFileContentRef(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return f(ctx, owner, repo, path, ref)
}

func TestHouseStyleContentsFallback(t *testing.T) {
	cfg := &config.Config{Project: config.ProjectConfig{Org: "org", PrimaryRepo: "repo", Repos: []string{"repo"}, Style: "Host rules"}}
	s := New(cfg, slog.Default())
	calls := 0
	s.SetStyleFetcher(styleFetchFunc(func(ctx context.Context, owner, repo, path, ref string) (string, error) {
		calls++
		if owner != "org" || repo != "repo" || path != "HIVE_STYLE.md" || ref != "" {
			t.Fatalf("wrong Contents request: %s %s %s %s", owner, repo, path, ref)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded Contents request")
		}
		return "Remote rules", nil
	}))
	if body, source := s.resolveHouseStyle("scanner"); body != "Remote rules" || source != "repo" {
		t.Fatalf("remote fallback: %q %s", body, source)
	}
	cfg.Project.CheckoutsDir = t.TempDir() // Wired but missing: no API fallback.
	if body, source := s.resolveHouseStyle("scanner"); body != "Host rules" || source != "hive" || calls != 1 {
		t.Fatalf("wired checkout should use host fallback: %q %s calls=%d", body, source, calls)
	}
	cfg.Project.CheckoutsDir = ""
	for _, f := range []promptsrc.Fetcher{fakeFetcher{err: errors.New("not found")}, fakeFetcher{body: strings.Repeat("x", maxHouseStyleBytes+1)}, fakeFetcher{body: mediumInjectionTitle}, nil} {
		s.SetStyleFetcher(f)
		if body, source := s.resolveHouseStyle("scanner"); body != "Host rules" || source != "hive" {
			t.Fatalf("invalid remote should use host: %q %s", body, source)
		}
	}
}

func TestHouseStylePolicyCheckoutAndPrimaryScope(t *testing.T) {
	root := t.TempDir()
	writeStyle(t, filepath.Join(root, "HIVE_STYLE.md"), "Policy repo style")
	cfg := &config.Config{Project: config.ProjectConfig{Org: "org", PrimaryRepo: "repo", Repos: []string{"repo"}}, Policies: config.PoliciesConfig{LocalDir: root, Repo: "https://github.com/org/repo.git"}}
	s := New(cfg, slog.Default())
	if body, source := s.resolveHouseStyle("scanner"); body != "Policy repo style" || source != "repo" {
		t.Fatalf("policy checkout ignored: %q %s", body, source)
	}
	cfg.Policies.Repo = "org/other"
	if _, source := s.resolveHouseStyle("scanner"); source != "built-in" {
		t.Fatal("style leaked from another repo")
	}
}

func TestHouseStyleEveryResolutionAndBackend(t *testing.T) {
	redirectPolicySeams(t)
	for _, backend := range []string{"claude", "codex", "copilot", "gemini"} {
		for _, path := range []string{"config", "remote", "embedded", "fallback"} {
			t.Run(backend+"/"+path, func(t *testing.T) {
				agent := config.AgentConfig{Role: "custom", Backend: backend}
				cfg := newPromptSourceCfg(t, agent, true, []string{"testorg/prompts"})
				cfg.Project.Style = "House rules sentinel"
				name := "gh-agent"
				switch path {
				case "remote":
					a := cfg.Agents[name]
					a.PromptSource = &config.PromptSourceConfig{Owner: "testorg", Repo: "prompts", Path: "custom.md"}
					cfg.Agents[name] = a
				case "embedded":
					name = "quality"
					cfg.Agents[name] = config.AgentConfig{Role: "quality", Backend: backend}
				case "fallback":
					name = "custom-agent"
					cfg.Agents[name] = agent
				}
				s := New(cfg, slog.Default())
				s.SetGitHubPromptResolver(promptsrc.NewResolver(fakeFetcher{body: "Remote policy"}, func(string) bool { return true }, nil))
				msg := s.BuildAgentMessage(name, nil, nil)
				header := crux.HouseStyleHeader("hive")
				if !strings.Contains(msg, "\n\n"+header+"\n\nHouse rules sentinel\n") || strings.Count(msg, header) != 1 {
					t.Fatalf("missing house style in %s/%s: %s", backend, path, msg)
				}
				first, _, _ := strings.Cut(msg, "\n")
				if !strings.HasPrefix(first, "[agent:") || !strings.HasPrefix(msg[len(first)+1:], "\n"+header) {
					t.Fatalf("style not directly under kick header: %s", msg)
				}
			})
		}
	}
	s := New(&config.Config{}, slog.Default())
	if got := s.assembleKick(kickAssembly{}, ""); got != "" {
		t.Fatal("house style resurrected a fail-closed kick")
	}
}

func TestHouseStyleScopedPrimaryAndLiveEdits(t *testing.T) {
	root := t.TempDir()
	writeStyle(t, filepath.Join(root, "primary", "HIVE_STYLE.md"), "Primary rules")
	scopedFile := filepath.Join(root, "scoped", "HIVE_STYLE.md")
	writeStyle(t, scopedFile, "Scoped rules")
	cfg := &config.Config{Project: config.ProjectConfig{Org: "org", PrimaryRepo: "primary", Repos: []string{"primary", "scoped"}, CheckoutsDir: root}, AgentRuntimeConfig: config.AgentRuntimeConfig{Agents: map[string]config.AgentConfig{"scanner": {Repos: []string{"scoped"}}}}}
	s := New(cfg, slog.Default())
	if body, source := s.resolveHouseStyle("scanner"); body != "Scoped rules" || source != "repo" {
		t.Fatalf("wrong primary style: %q %s", body, source)
	}
	writeStyle(t, scopedFile, "Updated rules")
	if body, _ := s.resolveHouseStyle("scanner"); body != "Updated rules" {
		t.Fatalf("style edit not picked up: %q", body)
	}
	if err := os.Remove(scopedFile); err != nil {
		t.Fatal(err)
	}
	cfg.Project.StyleFile = filepath.Join(root, "missing.md")
	cfg.Project.Style = "Host rules"
	if body, source := s.resolveHouseStyle("scanner"); body != "Host rules" || source != "hive" {
		t.Fatalf("missing files did not fall through: %q %s", body, source)
	}
}
