package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPastedWorkspacePathsImportsOnlyWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "report.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, imported, err := pastedWorkspacePaths(root, "file://"+file+"\n"+file)
	if err != nil || !imported || len(got) != 1 || got[0].meta.Filename != "report.txt" {
		t.Fatalf("got (%+v, %v, %v), want one imported report", got, imported, err)
	}
	if _, imported, err := pastedWorkspacePaths(root, "a normal text paste"); err != nil || imported {
		t.Fatalf("normal text was treated as attachment: imported=%v err=%v", imported, err)
	}
}

func TestWorkspaceAttachmentRejectsOutsideAndDirectories(t *testing.T) {
	root := t.TempDir()
	if _, err := workspaceAttachment(root, root); err == nil {
		t.Fatal("directory was accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := workspaceAttachment(root, outside); err == nil {
		t.Fatal("outside file was accepted")
	}
}

func TestImportDollarPathsQueuesExistingWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "diagram.png")
	if err := os.WriteFile(file, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, attachments, err := importDollarPaths(root, "see $diagram.png please")
	if err != nil || content != "see  please" || len(attachments) != 1 {
		t.Fatalf("got (%q, %+v, %v)", content, attachments, err)
	}
}
