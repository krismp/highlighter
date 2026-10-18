package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func videosDir(home string) string { return filepath.Join(home, "Movies") }

// Open opens a URL, folder or file with its default app.
func Open(target string) error { return exec.Command("open", target).Run() }

// Reveal shows a file selected in Finder.
func Reveal(path string) error { return exec.Command("open", "-R", path).Run() }

// Trash moves path to the Trash, where Finder's Put Back can restore it.
// macOS 14+ ships /usr/bin/trash; older versions ask Finder (macOS asks the
// user once to let Highlighter control Finder). Moving the file into ~/.Trash
// directly is not an option: privacy protection blocks that folder.
func Trash(path string) error {
	cmd := exec.Command("/usr/bin/trash", path)
	if _, err := os.Stat("/usr/bin/trash"); err != nil {
		cmd = exec.Command("osascript", "-e", `on run argv
tell application "Finder" to delete (POSIX file (item 1 of argv) as alias)
end run`, path)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("move to Trash: %s", strings.TrimSpace(string(out)+" "+err.Error()))
	}
	return nil
}
