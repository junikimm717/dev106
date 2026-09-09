package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junikimm717/dev106/internal/shared"
)

func TestFindRootGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FindRoot = %q, want %q", got, want)
	}
}

func TestFindRootGitFileWorktree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /tmp/somewhere\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FindRoot = %q, want %q", got, want)
	}
}

func TestFindRootMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := FindRoot(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "dev106 pull") {
		t.Fatalf("error should mention pull: %v", err)
	}
}

func TestContainerName(t *testing.T) {
	name, err := ContainerName("/tmp/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "dev106_") {
		t.Fatalf("name = %q", name)
	}
	if !strings.HasSuffix(name, "_"+RootID("/tmp/repo")) {
		t.Fatalf("name %q should end with hash of root", name)
	}
}

func TestContainerLabels(t *testing.T) {
	labels := ContainerLabels("/Users/me/xv6")
	if labels[shared.LabelManaged] != "true" {
		t.Fatalf("managed label = %q", labels[shared.LabelManaged])
	}
	if labels[shared.LabelRoot] != "/Users/me/xv6" {
		t.Fatalf("root label = %q", labels[shared.LabelRoot])
	}
	if RootID(labels[shared.LabelRoot]) != RootID("/Users/me/xv6") {
		t.Fatal("root label must reconstruct the same hash")
	}
}
