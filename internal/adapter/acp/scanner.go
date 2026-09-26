package acp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

const codexACPVersion = "1.13.1"

type CandidateSpec struct {
	Key         string
	DisplayName string
	Command     string
	Args        []string
}

type Scanner struct {
	candidates      []CandidateSpec
	codexInstallDir string
	installMu       sync.Mutex
	now             func() time.Time
}

var _ application.CandidateScanner = (*Scanner)(nil)

func NewScanner(candidates []CandidateSpec, now func() time.Time) *Scanner {
	return &Scanner{candidates: candidates, now: now}
}

func NewCodexScanner(dataDir string, now func() time.Time) *Scanner {
	return &Scanner{
		codexInstallDir: filepath.Join(dataDir, "agents", "codex-acp", codexACPVersion),
		now:             now,
	}
}

func (s *Scanner) List(ctx context.Context, _ bool) (application.AgentCandidateResult, error) {
	candidates := s.candidates
	if s.codexInstallDir != "" {
		s.installMu.Lock()
		candidate, err := ensureCodexACP(ctx, s.codexInstallDir)
		s.installMu.Unlock()

		if err != nil {
			return application.AgentCandidateResult{}, err
		}

		candidates = []CandidateSpec{candidate}
	}

	items := make([]application.AgentCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return application.AgentCandidateResult{}, err
		}

		path, err := exec.LookPath(candidate.Command)
		if err != nil {
			continue
		}

		items = append(items, application.AgentCandidate{
			CandidateKey: candidate.Key, DisplayName: candidate.DisplayName, Command: candidate.Command,
			Args: append([]string(nil), candidate.Args...), Transport: "stdio", Source: "path",
			ResolvedExecutablePath: path, Warnings: []string{},
		})
	}

	return application.AgentCandidateResult{Items: items, ScannedAt: s.now(), Warnings: []string{}}, nil
}

func ensureCodexACP(ctx context.Context, installDir string) (CandidateSpec, error) {
	nodePath, err := findExecutable("node", "/opt/homebrew/bin/node", "/usr/local/bin/node")
	if err != nil {
		return CandidateSpec{}, dependencyError("Node.jsが見つかりません。")
	}

	entrypoint := filepath.Join(
		installDir,
		"node_modules",
		"@agentclientprotocol",
		"codex-acp",
		"dist",
		"index.js",
	)
	if _, err := os.Stat(entrypoint); err == nil {
		return codexCandidate(nodePath, entrypoint), nil
	}

	npmPath, err := findExecutable("npm", "/opt/homebrew/bin/npm", "/usr/local/bin/npm")
	if err != nil {
		return CandidateSpec{}, dependencyError("npmが見つかりません。")
	}

	npmCLI, err := filepath.EvalSymlinks(npmPath)
	if err != nil {
		return CandidateSpec{}, dependencyError("npmを起動できません。")
	}

	packageSpec := "@agentclientprotocol/codex-acp@" + codexACPVersion

	command := exec.CommandContext(
		ctx,
		nodePath,
		npmCLI,
		"install",
		"--prefix",
		installDir,
		"--no-save",
		"--package-lock=false",
		"--omit=dev",
		"--no-audit",
		"--no-fund",
		packageSpec,
	)
	if err := command.Run(); err != nil {
		return CandidateSpec{}, dependencyError("Codex ACPをインストールできませんでした。")
	}

	return codexCandidate(nodePath, entrypoint), nil
}

func codexCandidate(nodePath, entrypoint string) CandidateSpec {
	return CandidateSpec{
		Key: "codex-acp", DisplayName: "Codex ACP", Command: nodePath, Args: []string{entrypoint},
	}
}

func findExecutable(name string, fallbacks ...string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	for _, path := range fallbacks {
		if resolved, err := exec.LookPath(path); err == nil {
			return resolved, nil
		}
	}

	return "", exec.ErrNotFound
}

func dependencyError(message string) error {
	return &shared.Error{Code: "dependency_install_failed", Message: message, Retryable: true}
}
