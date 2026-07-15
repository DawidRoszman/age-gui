package view

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Platform is the set of desktop capabilities the handlers need.
//
// Handlers depend on this interface rather than calling the Wails runtime
// directly, which keeps them unit-testable with a fake: the runtime functions
// need a live window and would otherwise make every handler test require a
// display and a GTK build.
type Platform interface {
	// OpenFileDialog asks the user to pick a file. It returns "" when the
	// dialog was cancelled, which is a normal outcome and not an error.
	OpenFileDialog(title string) (string, error)
	// OpenFilesDialog asks the user to pick one or more files.
	OpenFilesDialog(title string) ([]string, error)
	// SaveFileDialog asks where to write a file. Returns "" when cancelled.
	SaveFileDialog(title, defaultName string) (string, error)
	// SetClipboard copies text to the system clipboard.
	SetClipboard(text string) error
	// EmitEvent sends an event to the frontend.
	EmitEvent(name string, data ...any)
}

// WailsPlatform implements Platform against the Wails runtime.
//
// The Wails runtime keys everything off the startup context, so this holds it
// and hands it to each call. That is why SetContext exists: the context is only
// available once the app has started, well after the handlers are wired.
type WailsPlatform struct {
	ctx context.Context
}

// NewWailsPlatform returns a platform whose context must be set at startup.
func NewWailsPlatform() *WailsPlatform { return &WailsPlatform{} }

// SetContext supplies the Wails startup context. Call from OnStartup before
// any handler runs.
func (p *WailsPlatform) SetContext(ctx context.Context) { p.ctx = ctx }

// OpenFileDialog implements Platform.
func (p *WailsPlatform) OpenFileDialog(title string) (string, error) {
	return runtime.OpenFileDialog(p.ctx, runtime.OpenDialogOptions{Title: title})
}

// OpenFilesDialog implements Platform.
func (p *WailsPlatform) OpenFilesDialog(title string) ([]string, error) {
	return runtime.OpenMultipleFilesDialog(p.ctx, runtime.OpenDialogOptions{Title: title})
}

// SaveFileDialog implements Platform.
func (p *WailsPlatform) SaveFileDialog(title, defaultName string) (string, error) {
	return runtime.SaveFileDialog(p.ctx, runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultName,
	})
}

// SetClipboard implements Platform.
func (p *WailsPlatform) SetClipboard(text string) error {
	return runtime.ClipboardSetText(p.ctx, text)
}

// EmitEvent implements Platform.
func (p *WailsPlatform) EmitEvent(name string, data ...any) {
	runtime.EventsEmit(p.ctx, name, data...)
}

var _ Platform = (*WailsPlatform)(nil)
