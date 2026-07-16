package view

import (
	"dawidroszman.eu/encryptor/internal/service"
)

// Groups is the Wails-bound handler for contact groups.
type Groups struct {
	groups *service.GroupService
}

// NewGroups builds the handler.
func NewGroups(groups *service.GroupService) *Groups {
	return &Groups{groups: groups}
}

// List returns every group, sorted by name.
func (h *Groups) List() GroupsResult {
	all, err := h.groups.List()
	if err != nil {
		return GroupsResult{Groups: []GroupDTO{}, Error: mapError(err)}
	}
	return GroupsResult{Groups: groupDTOs(all)}
}

// Create saves a new group with the given members.
func (h *Groups) Create(name string, memberIDs []string) GroupResult {
	g, err := h.groups.Create(name, memberIDs)
	if err != nil {
		return GroupResult{Error: mapError(err)}
	}
	return GroupResult{Group: groupDTO(g)}
}

// Update replaces a group's name and members.
func (h *Groups) Update(id, name string, memberIDs []string) GroupResult {
	g, err := h.groups.Update(id, name, memberIDs)
	if err != nil {
		return GroupResult{Error: mapError(err)}
	}
	return GroupResult{Group: groupDTO(g)}
}

// Delete removes a group.
func (h *Groups) Delete(id string) VoidResult {
	if err := h.groups.Delete(id); err != nil {
		return VoidResult{Error: mapError(err)}
	}
	return VoidResult{}
}
