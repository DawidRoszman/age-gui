package view

import (
	"dawidroszman.eu/age-gui/internal/service"
)

// Contacts is the Wails-bound handler for the address book.
type Contacts struct {
	contacts *service.ContactService
	platform Platform
}

// NewContacts builds the handler.
func NewContacts(contacts *service.ContactService, platform Platform) *Contacts {
	return &Contacts{contacts: contacts, platform: platform}
}

// List returns every contact, sorted by name.
func (h *Contacts) List() ContactsResult {
	all, err := h.contacts.List()
	if err != nil {
		return ContactsResult{Contacts: []ContactDTO{}, Error: mapError(err)}
	}
	return ContactsResult{Contacts: contactDTOs(all)}
}

// Add saves a contact from a pasted public key.
func (h *Contacts) Add(name, publicKey, note string) ContactResult {
	c, err := h.contacts.Add(name, publicKey, note)
	if err != nil {
		return ContactResult{Error: mapError(err)}
	}
	return ContactResult{Contact: contactDTO(c)}
}

// ImportFromFile asks for a public key file and saves it as a contact.
//
// Returns an empty result with no error when the dialog is cancelled; the UI
// distinguishes that from success by the empty ID.
func (h *Contacts) ImportFromFile(name, note string) ContactResult {
	path, err := h.platform.OpenFileDialog("Choose a public key file")
	if err != nil {
		return ContactResult{Error: mapError(err)}
	}
	if path == "" {
		return ContactResult{} // cancelled
	}

	c, err := h.contacts.AddFromFile(name, path, note)
	if err != nil {
		return ContactResult{Error: mapError(err)}
	}
	return ContactResult{Contact: contactDTO(c)}
}

// Rename updates a contact's name and note. The public key is immutable.
func (h *Contacts) Rename(id, name, note string) ContactResult {
	c, err := h.contacts.Rename(id, name, note)
	if err != nil {
		return ContactResult{Error: mapError(err)}
	}
	return ContactResult{Contact: contactDTO(c)}
}

// Delete removes a contact.
func (h *Contacts) Delete(id string) VoidResult {
	if err := h.contacts.Delete(id); err != nil {
		return VoidResult{Error: mapError(err)}
	}
	return VoidResult{}
}

// CopyPublicKey puts a contact's public key on the clipboard, for passing on to
// someone else.
func (h *Contacts) CopyPublicKey(id string) VoidResult {
	c, err := h.contacts.Get(id)
	if err != nil {
		return VoidResult{Error: mapError(err)}
	}
	if err := h.platform.SetClipboard(c.PublicKey.String()); err != nil {
		return VoidResult{Error: mapError(err)}
	}
	return VoidResult{}
}
