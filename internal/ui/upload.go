package ui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"awesomeProject/internal/store"
)

// MaxUploadBytes is Discord's standard per-request upload ceiling. Servers
// with larger boosts can accept more, but using the portable limit prevents a
// failed request after the composer has been cleared.
const MaxUploadBytes int64 = 25 * 1024 * 1024

type queuedAttachment struct {
	path string
	meta store.Attachment
	temp bool
}

func workspaceAttachment(workspace, name string) (queuedAttachment, error) {
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return queuedAttachment{}, fmt.Errorf("resolve workspace: %w", err)
	}
	path, err := filepath.Abs(name)
	if err != nil {
		return queuedAttachment{}, err
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return queuedAttachment{}, fmt.Errorf("read attachment: %w", err)
	}
	rel, err := filepath.Rel(root, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return queuedAttachment{}, fmt.Errorf("attachment must be inside the workspace")
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return queuedAttachment{}, fmt.Errorf("read attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return queuedAttachment{}, fmt.Errorf("attachment must be a regular file")
	}
	if info.Size() > MaxUploadBytes {
		return queuedAttachment{}, fmt.Errorf("attachment %q is larger than the %d MiB upload limit", info.Name(), MaxUploadBytes/(1024*1024))
	}
	file, err := os.Open(realPath)
	if err != nil {
		return queuedAttachment{}, fmt.Errorf("read attachment: %w", err)
	}
	_ = file.Close()
	return queuedAttachment{path: realPath, meta: store.Attachment{Filename: info.Name(), Size: info.Size()}}, nil
}

// pastedWorkspacePaths returns files only when every non-empty pasted line is
// an absolute workspace path or file URI. This keeps ordinary pasted prose in
// the composer instead of unexpectedly treating it as an upload.
func pastedWorkspacePaths(workspace, pasted string) ([]queuedAttachment, bool, error) {
	lines := strings.Fields(strings.TrimSpace(pasted))
	if len(lines) == 0 {
		return nil, false, nil
	}
	attachments := make([]queuedAttachment, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		path := line
		if strings.HasPrefix(line, "file://") {
			u, err := url.Parse(line)
			if err != nil || u.Scheme != "file" || u.Host != "" {
				return nil, false, nil
			}
			path, err = url.PathUnescape(u.Path)
			if err != nil {
				return nil, false, nil
			}
		}
		if !filepath.IsAbs(path) {
			return nil, false, nil
		}
		attachment, err := workspaceAttachment(workspace, path)
		if err != nil {
			return nil, false, err
		}
		if !seen[attachment.path] {
			seen[attachment.path] = true
			attachments = append(attachments, attachment)
		}
	}
	return attachments, true, nil
}

// importDollarPaths replaces standalone $path tokens with queued workspace
// files. It only consumes tokens that resolve to regular files, so shell-like
// prose and currency remain ordinary message text. The picker uses the same
// replacement semantics after a fuzzy selection.
func importDollarPaths(workspace, content string) (string, []queuedAttachment, error) {
	words := strings.Fields(content)
	if len(words) == 0 {
		return content, nil, nil
	}
	attachments := make([]queuedAttachment, 0)
	seen := map[string]bool{}
	for _, word := range words {
		if !strings.HasPrefix(word, "$") || len(word) == 1 {
			continue
		}
		path := strings.TrimPrefix(word, "$")
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspace, path)
		}
		attachment, err := workspaceAttachment(workspace, path)
		if err != nil {
			continue
		}
		if !seen[attachment.path] {
			seen[attachment.path] = true
			attachments = append(attachments, attachment)
		}
		content = strings.Replace(content, word, "", 1)
	}
	return strings.TrimSpace(content), attachments, nil
}
