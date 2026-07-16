package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The localized case is the whole reason this code parses a config file instead
// of joining "Downloads" onto the home directory. Getting it wrong does not
// error -- it silently creates a second folder beside the real one and drops
// the user's files into it.
func TestXDGDownloadDir_ReadsLocalizedFolderFromConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG user-dirs is a Linux convention")
	}
	home := t.TempDir()
	writeUserDirs(t, home, `# This file is written by xdg-user-dirs-update
XDG_DESKTOP_DIR="$HOME/Pulpit"
XDG_DOWNLOAD_DIR="$HOME/Pobrane"
XDG_MUSIC_DIR="$HOME/Muzyka"
`)

	if got, want := xdgDownloadDir(home), filepath.Join(home, "Pobrane"); got != want {
		t.Errorf("xdgDownloadDir = %q, want %q", got, want)
	}
}

func TestXDGDownloadDir_EnvironmentWins(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG user-dirs is a Linux convention")
	}
	home := t.TempDir()
	writeUserDirs(t, home, `XDG_DOWNLOAD_DIR="$HOME/FromFile"`+"\n")
	t.Setenv("XDG_DOWNLOAD_DIR", filepath.Join(home, "FromEnv"))

	if got, want := xdgDownloadDir(home), filepath.Join(home, "FromEnv"); got != want {
		t.Errorf("xdgDownloadDir = %q, want %q -- the environment must win", got, want)
	}
}

func TestXDGDownloadDir_MissingConfigIsNotAnError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG user-dirs is a Linux convention")
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "nonexistent"))
	t.Setenv("XDG_DOWNLOAD_DIR", "")

	// "" means "I don't know", which DownloadsDir turns into the fallback.
	if got := xdgDownloadDir(home); got != "" {
		t.Errorf("xdgDownloadDir = %q, want empty when there is no config", got)
	}
}

// A relative value cannot be honoured: the working directory of a
// double-clicked app is arbitrary, so the fallback is the safer answer.
func TestXDGDownloadDir_RelativeValueIsRejected(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG user-dirs is a Linux convention")
	}
	home := t.TempDir()
	writeUserDirs(t, home, `XDG_DOWNLOAD_DIR="somewhere/relative"`+"\n")
	t.Setenv("XDG_DOWNLOAD_DIR", "")

	if got := xdgDownloadDir(home); got != "" {
		t.Errorf("xdgDownloadDir = %q, want empty for a relative path", got)
	}
}

func TestDownloadsDir_FallsBackWhenNothingIsConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "nonexistent"))

	got, err := DownloadsDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "Downloads"); got != want {
		t.Errorf("DownloadsDir = %q, want %q", got, want)
	}
}

// The folder is a default, not a promise that it exists: a fresh account may
// have no Downloads folder at all, and refusing to start over that would be
// absurd. Whoever writes there creates it.
func TestDownloadsDir_DoesNotRequireTheFolderToExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "nonexistent"))

	got, err := DownloadsDir()
	if err != nil {
		t.Fatalf("DownloadsDir errored on a home with no Downloads folder: %v", err)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s should not exist", got)
	}
}

func writeUserDirs(t *testing.T, home, content string) {
	t.Helper()
	cfg := filepath.Join(home, ".config")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, userDirsFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DOWNLOAD_DIR", "")
}
