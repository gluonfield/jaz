package storage

type WorktreeRegistration struct {
	Path, Project, Branch string
	MergedHead            string `json:",omitempty"`
}
