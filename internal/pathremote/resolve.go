package pathremote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/seif/token-usage-service/internal/model"
)

// Resolve walks up from sourcePath to find a git repo and origin remote URL.
func Resolve(hostID, sourcePath string) model.PathRemote {
	pr := model.PathRemote{
		HostID:     hostID,
		SourcePath: sourcePath,
		RemoteName: "origin",
	}
	dir := sourcePath
	if fi, err := os.Stat(sourcePath); err == nil && !fi.IsDir() {
		dir = filepath.Dir(sourcePath)
	}
	// Prefer cwd from parent project when source is under ~/.claude — still try git -C
	root, err := gitTopLevel(dir)
	if err != nil || root == "" {
		return pr
	}
	pr.RepoRoot = &root
	url, err := gitRemoteURL(root, "origin")
	if err != nil || url == "" {
		return pr
	}
	pr.RemoteURL = &url
	return pr
}

// ResolveWithKnownRemote uses a remote URL already embedded in the transcript
// (e.g. Codex's session_meta.git.repository_url) when available, skipping the
// local git shell-out entirely. This is more reliable than ResolveFromCWD
// since it works even when ingest runs on a different machine than the one
// that recorded the session, or the repo no longer exists locally.
func ResolveWithKnownRemote(hostID, sourcePath, cwd, knownRemoteURL string) model.PathRemote {
	if knownRemoteURL != "" {
		url := normalizeRemoteURL(knownRemoteURL)
		return model.PathRemote{
			HostID:     hostID,
			SourcePath: sourcePath,
			RemoteName: "origin",
			RemoteURL:  &url,
		}
	}
	return ResolveFromCWD(hostID, sourcePath, cwd)
}

// ResolveFromCWD resolves remote using a workspace cwd (preferred for transcript metadata).
func ResolveFromCWD(hostID, sourcePath, cwd string) model.PathRemote {
	if cwd != "" {
		if root, err := gitTopLevel(cwd); err == nil && root != "" {
			pr := model.PathRemote{
				HostID:     hostID,
				SourcePath: sourcePath,
				RemoteName: "origin",
				RepoRoot:   &root,
			}
			if url, err := gitRemoteURL(root, "origin"); err == nil && url != "" {
				pr.RemoteURL = &url
			}
			return pr
		}
	}
	return Resolve(hostID, sourcePath)
}

func gitTopLevel(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRemoteURL(repoRoot, name string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "remote", "get-url", name)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return normalizeRemoteURL(strings.TrimSpace(string(out))), nil
}

// normalizeRemoteURL canonicalizes a git remote URL so the same repository
// resolves to the same string regardless of whether it was read via
// `git remote get-url` (which may return the SSH form with a `.git` suffix,
// e.g. git@github.com:org/repo.git) or from transcript metadata such as
// Codex's session_meta.git.repository_url (typically an https URL with no
// `.git` suffix, e.g. https://github.com/org/repo).
func normalizeRemoteURL(raw string) string {
	url := strings.TrimSpace(raw)
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, ".git")
	if rest, ok := strings.CutPrefix(url, "git@"); ok {
		if host, path, ok := strings.Cut(rest, ":"); ok {
			url = "https://" + host + "/" + path
		}
	} else if rest, ok := strings.CutPrefix(url, "ssh://git@"); ok {
		url = "https://" + rest
	}
	return url
}
