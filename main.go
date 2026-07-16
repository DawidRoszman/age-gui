// Command encryptor is a desktop wrapper around age, so people who don't use a
// terminal can still share secrets securely.
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"dawidroszman.eu/encryptor/internal/service"
	"dawidroszman.eu/encryptor/internal/storage"
	"dawidroszman.eu/encryptor/internal/view"
)

//go:embed all:frontend/dist
var assets embed.FS

// EventFilesDropped carries paths dropped onto the window.
const EventFilesDropped = "files:dropped"

// autoLockInterval is how often the idle check runs. It is granularity, not the
// timeout: a 15 minute auto-lock fires within 15 seconds of the deadline, which
// nobody will notice, while a tighter tick would wake the CPU for nothing.
const autoLockInterval = 15 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatalf("encryptor: %v", err)
	}
}

// run is the composition root: it builds the layers bottom-up and wires them
// together. This is the only place that knows how the pieces fit, which is what
// keeps every layer below testable in isolation.
func run() error {
	dir, err := storage.DefaultDir()
	if err != nil {
		return fmt.Errorf("prepare config directory: %w", err)
	}

	// Storage adapters implement the ports declared in the service package.
	identityStore := storage.NewIdentity(dir)
	contactStore := storage.NewContacts(dir)
	settingsStore := storage.NewSettings(dir)

	// Services. Note no options are passed: production always gets age's full
	// scrypt work factor and the real clock. The test-only knobs are unexported
	// and unreachable from here by construction.
	keySvc := service.NewKeyService(identityStore)
	contactSvc := service.NewContactService(contactStore)
	cryptoSvc := service.NewCryptoService(keySvc)

	// Where output goes when the user has not chosen a folder. Resolved once
	// here rather than per operation: it cannot change while the app runs, and
	// the service layer has no business knowing how an OS names this folder.
	//
	// A failure here means we could not even locate a home directory. Falling
	// back to the working directory would scatter files somewhere the user
	// would not think to look, so refuse to start and say why.
	downloads, err := storage.DownloadsDir()
	if err != nil {
		return fmt.Errorf("locate downloads folder: %w", err)
	}

	// Constructing this applies the stored auto-lock preference, so a setting
	// chosen weeks ago is in force before the window opens.
	settingsSvc, err := service.NewSettingsService(settingsStore, keySvc, downloads)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	// The platform adapter needs the Wails context, which does not exist until
	// startup, so it is created empty and filled in OnStartup below.
	platform := view.NewWailsPlatform()

	keysHandler := view.NewKeys(keySvc, platform)
	contactsHandler := view.NewContacts(contactSvc, platform)
	cryptoHandler := view.NewCrypto(cryptoSvc, contactSvc, settingsSvc, platform)
	settingsHandler := view.NewSettings(settingsSvc, platform)

	// Tell the UI when an idle lock happens, so it can move the user to the
	// unlock screen rather than leave them on controls that quietly stopped
	// working. The service cannot do this itself: it must not know about a GUI.
	keySvc.SetAutoLockHandler(func() {
		platform.EmitEvent(view.EventAutoLocked)
	})

	return wails.Run(&options.App{
		Title:  "Encryptor",
		Width:  1000,
		Height: 700,
		// A hard floor: the sidebar plus content stops being usable below this,
		// and a window the user cannot read is worse than one they must resize.
		MinWidth:  800,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		DragAndDrop: &options.DragAndDrop{
			// Wails then hands OnFileDrop real absolute paths, which is what
			// lets encryption stream in Go instead of pulling bytes into the
			// webview.
			EnableFileDrop: true,
			// The webview's own drop handling would try to navigate to the
			// dropped file, replacing the UI with a file view.
			DisableWebViewDrop: true,
		},
		Linux: &linux.Options{
			ProgramName: "Encryptor",
		},
		OnStartup: func(ctx context.Context) {
			platform.SetContext(ctx)

			// Forward drops to the frontend as a normal event. The paths are
			// absolute, so the frontend passes them straight back to the
			// crypto handler without ever touching file contents.
			wailsruntime.OnFileDrop(ctx, func(_, _ int, paths []string) {
				platform.EmitEvent(EventFilesDropped, paths)
			})

			// The idle watcher lives for the app's lifetime and stops with the
			// context. autoLockInterval only bounds how late a lock can be, so
			// a coarse tick costs nothing and wakes the CPU rarely.
			go keySvc.StartAutoLock(ctx, autoLockInterval)
		},
		Bind: []any{
			keysHandler,
			contactsHandler,
			cryptoHandler,
			settingsHandler,
		},
		EnumBind: []any{},
	})
}
