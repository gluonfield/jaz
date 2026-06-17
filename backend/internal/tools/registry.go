package tools

import "sync"

type Registry struct {
	mu      sync.RWMutex
	ordered []Tool
	byName  map[string]Tool
	groups  map[string][]string
}

func NewRegistry(toolList ...Tool) *Registry {
	r := &Registry{byName: make(map[string]Tool), groups: make(map[string][]string)}
	for _, tool := range toolList {
		r.Add(tool)
	}
	return r
}

func (r *Registry) Add(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addLocked(tool)
}

func (r *Registry) addLocked(tool Tool) string {
	def := tool.Definition()
	name := DefinitionName(def)
	r.ordered = append(r.ordered, tool)
	r.byName[name] = tool
	return name
}

func (r *Registry) SetGroup(group string, toolList []Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeGroupLocked(group)
	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, r.addLocked(tool))
	}
	r.groups[group] = names
}

func (r *Registry) RemoveGroup(group string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeGroupLocked(group)
}

func (r *Registry) removeGroupLocked(group string) {
	names := r.groups[group]
	if len(names) == 0 {
		return
	}
	remove := make(map[string]bool, len(names))
	for _, name := range names {
		remove[name] = true
		delete(r.byName, name)
	}
	ordered := make([]Tool, 0, len(r.ordered)-len(remove))
	for _, tool := range r.ordered {
		if remove[DefinitionName(tool.Definition())] {
			continue
		}
		ordered = append(ordered, tool)
	}
	r.ordered = ordered
	delete(r.groups, group)
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.byName[name]
	return tool, ok
}

func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]Definition, 0, len(r.ordered))
	for _, tool := range r.ordered {
		defs = append(defs, tool.Definition())
	}
	return defs
}

func (r *Registry) DefinitionsWhere(include func(string) bool) []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]Definition, 0, len(r.ordered))
	for _, tool := range r.ordered {
		def := tool.Definition()
		if include == nil || include(DefinitionName(def)) {
			defs = append(defs, def)
		}
	}
	return defs
}
