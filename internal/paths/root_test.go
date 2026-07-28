package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canonical(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolve %s: %v", dir, err)
	}
	return resolved
}

func mkdirAll(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	return dir
}

func TestProjectRootPrefersTheNearestProjectConfig(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	root := canonical(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, GitDirName))
	nested := mkdirAll(t, filepath.Join(root, "services", "api"))
	mkdirAll(t, filepath.Join(nested, ProjectDirName))
	if err := os.WriteFile(filepath.Join(nested, ProjectDirName, ConfigFileName), []byte("music:\n  root: A\n"), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	got, err := ProjectRoot(filepath.Join(nested, "internal"))
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}

	if got != nested {
		t.Errorf("ProjectRoot = %q, want the directory owning the nearest .hum/config.yaml (%q)", got, nested)
	}
}

func TestProjectRootFallsBackToTheGitRoot(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	root := canonical(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, GitDirName))
	nested := mkdirAll(t, filepath.Join(root, "cmd", "hum"))

	got, err := ProjectRoot(nested)
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}

	if got != root {
		t.Errorf("ProjectRoot = %q, want the git root %q", got, root)
	}
}

func TestProjectRootAcceptsAGitFileFromAWorktree(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	root := canonical(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(root, GitDirName), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	nested := mkdirAll(t, filepath.Join(root, "pkg"))

	got, err := ProjectRoot(nested)
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}

	if got != root {
		t.Errorf("ProjectRoot = %q, want the worktree root %q", got, root)
	}
}

func TestProjectRootFallsBackToTheStartDirectory(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	dir := canonical(t, t.TempDir())

	got, err := ProjectRoot(dir)
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}

	if got != dir {
		t.Errorf("ProjectRoot = %q, want the start directory %q when nothing marks a project", got, dir)
	}
}

func TestProjectRootResolvesSymlinks(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	real := canonical(t, t.TempDir())
	mkdirAll(t, filepath.Join(real, GitDirName))
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := ProjectRoot(link)
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}

	if got != real {
		t.Errorf("ProjectRoot = %q, want the canonical path %q so a session is not double-counted", got, real)
	}
}

func TestProjectRootDefaultsToTheWorkingDirectory(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	dir := canonical(t, t.TempDir())
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := ProjectRoot("")
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}

	if got != dir {
		t.Errorf("ProjectRoot(\"\") = %q, want the working directory %q", got, dir)
	}
}

func TestProjectRootReportsAMissingDirectory(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	absent := filepath.Join(t.TempDir(), "absent")

	_, err := ProjectRoot(absent)

	if err == nil {
		t.Fatal("ProjectRoot on a missing directory = nil, want an error rather than a silent fallback")
	}
	if !strings.Contains(err.Error(), absent) {
		t.Errorf("error = %q, want it to name %q", err, absent)
	}
}

func TestProjectRootReportsAnUnresolvableWorkingDirectory(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	original := absolute
	t.Cleanup(func() { absolute = original })
	absolute = func(string) (string, error) { return "", os.ErrNotExist }

	if _, err := ProjectRoot("."); err == nil {
		t.Fatal("ProjectRoot with an unresolvable path = nil, want an error")
	}
}
