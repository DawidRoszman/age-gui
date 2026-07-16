package storage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// downloadsFallback is the folder name to use when nothing else identifies one.
const downloadsFallback = "Downloads"

// userDirsFile is where freedesktop records the user's real folder names,
// relative to the config directory.
const userDirsFile = "user-dirs.dirs"

// DownloadsDir returns the directory the OS uses for downloaded files.
//
// This is the default landing place for encrypted and decrypted output. It is
// resolved once at startup rather than stored in settings, so the app follows
// the folder if the user relocates it.
//
// The directory is not required to exist; whoever writes there creates it.
func DownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}

	// macOS and Windows both fix this folder's location, and neither
	// translates the path itself -- a Polish macOS still writes to ~/Downloads
	// and shows "Pobrane" only in Finder. Linux is the exception, below.
	if runtime.GOOS != "linux" {
		return filepath.Join(home, downloadsFallback), nil
	}

	if dir := xdgDownloadDir(home); dir != "" {
		return dir, nil
	}
	return filepath.Join(home, downloadsFallback), nil
}

// xdgDownloadDir reads the freedesktop user-dirs configuration, returning ""
// when it cannot be determined.
//
// This matters on localized desktops: the folder is "Pobrane" on a Polish
// system and "Téléchargements" on a French one. Guessing ~/Downloads there
// would not merely be untidy, it would create a second folder next to the real
// one and drop files into a place the file manager does not show as Downloads.
//
// The environment variable is checked first but is usually absent -- desktop
// environments write the value to the config file and only export it into
// sessions started from certain launchers.
func xdgDownloadDir(home string) string {
	if dir := os.Getenv("XDG_DOWNLOAD_DIR"); dir != "" {
		return expandHome(dir, home)
	}

	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}

	f, err := os.Open(filepath.Join(base, userDirsFile))
	if err != nil {
		return "" // no config: the caller falls back
	}
	defer f.Close()

	// The format is shell-like: XDG_DOWNLOAD_DIR="$HOME/Downloads", with
	// comments. It is not worth a shell to evaluate; only $HOME is ever used
	// in practice, and a wrong guess here is corrected by the fallback.
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "XDG_DOWNLOAD_DIR=") {
			continue
		}
		value := strings.TrimPrefix(line, "XDG_DOWNLOAD_DIR=")
		value = strings.Trim(value, `"`)
		if value == "" {
			continue
		}
		return expandHome(value, home)
	}
	return ""
}

// expandHome resolves the only variable freedesktop actually uses here.
func expandHome(dir, home string) string {
	if after, ok := strings.CutPrefix(dir, "$HOME"); ok {
		return filepath.Join(home, after)
	}
	if after, ok := strings.CutPrefix(dir, "${HOME}"); ok {
		return filepath.Join(home, after)
	}
	// A relative path here would be meaningless to us, and the fallback is
	// better than writing somewhere arbitrary.
	if !filepath.IsAbs(dir) {
		return ""
	}
	return dir
}
