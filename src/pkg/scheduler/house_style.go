package scheduler

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hivecommons/hive/crux"
	"github.com/hivecommons/hive/pkg/config"
	"github.com/hivecommons/hive/pkg/promptsrc"
)

const maxHouseStyleBytes = 4096

const builtInHouseStyle = `Make the smallest complete diff. Avoid speculative abstractions. Follow existing repository patterns. Add tests for behaviour changes. Sign off every commit with git commit -s (DCO).`

// SetStyleFetcher supplies the Contents API client for the configured primary
// repo only. An empty ref asks GitHub for the default branch. Unlike arbitrary
// prompt_source URLs, the repository identity comes from project configuration.
func (s *Scheduler) SetStyleFetcher(f promptsrc.Fetcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.styleFetcher = f
}

// readHouseStyle bounds local reads too; max+1 distinguishes an oversized file
// from one exactly at the cap without loading an unbounded host file.
func readHouseStyle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	body, err := io.ReadAll(io.LimitReader(f, maxHouseStyleBytes+1))
	if err != nil {
		return ""
	}
	return string(body)
}

func validHouseStyle(body string) bool {
	return len(body) <= maxHouseStyleBytes && strings.TrimSpace(body) != ""
}

func (s *Scheduler) resolveHouseStyle(agent string) (body, source string) {
	if s.cfg == nil {
		return builtInHouseStyle, "built-in"
	}
	repo := s.cfg.PrimaryRepoForAgent(agent)
	if root := s.agentsRepoRoot(repo); root != "" {
		body = readHouseStyle(filepath.Join(root, "HIVE_STYLE.md"))
	} else if s.cfg.ForgeKindForRepo(repo) == config.ForgeGitHub {
		s.mu.RLock()
		fetcher := s.styleFetcher
		s.mu.RUnlock()
		owner, name, ok := strings.Cut(config.QualifyRepo(s.cfg.Project.Org, repo), "/")
		if fetcher != nil && ok && owner != "" && name != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			text, err := fetcher.GetFileContentRef(ctx, owner, name, "HIVE_STYLE.md", "")
			cancel()
			if err == nil {
				body = text
			}
		}
	}
	if validHouseStyle(body) {
		text, verdict := s.enforceIssueTextVerdict(body)
		if !verdict.Blocked {
			return text, "repo"
		}
		// Never echo refused instructions into audit logs or agent prompts.
		detail := fmt.Sprintf("repo=%s file=HIVE_STYLE.md reason=ioscan_block", config.QualifyRepo(s.cfg.Project.Org, repo))
		if audit := s.auditLogger(); audit != nil {
			audit("style_refused", detail, agent)
		}
		s.warnOnce("style_refused:"+repo, "style_refused", "repo", repo, "reason", "ioscan_block")
	}
	if path := strings.TrimSpace(s.cfg.Project.StyleFile); path != "" {
		if text := readHouseStyle(path); validHouseStyle(text) {
			return text, "hive"
		}
	}
	if text := s.cfg.Project.Style; validHouseStyle(text) {
		return text, "hive"
	}
	return builtInHouseStyle, "built-in"
}

func (s *Scheduler) addHouseStyle(agent, message string) string {
	body, source := s.resolveHouseStyle(agent)
	return insertAfterKickHeader(message, crux.HouseStyleHeader(source)+"\n\n"+strings.TrimSpace(body))
}
