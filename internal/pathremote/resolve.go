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
	return strings.TrimSpace(string(out)), nil
}
