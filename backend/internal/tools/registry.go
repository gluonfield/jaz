package tools

type Registry struct {
	ordered []Tool
	byName  map[string]Tool
}

func NewRegistry(toolList ...Tool) *Registry {
	r := &Registry{byName: make(map[string]Tool)}
	for _, tool := range toolList {
		r.Add(tool)
	}
	return r
}

func (r *Registry) Add(tool Tool) {
	def := tool.Definition()
	r.ordered = append(r.ordered, tool)
	r.byName[DefinitionName(def)] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.byName[name]
	return tool, ok
}

func (r *Registry) Definitions() []Definition {
	defs := make([]Definition, 0, len(r.ordered))
	for _, tool := range r.ordered {
		defs = append(defs, tool.Definition())
	}
	return defs
}
