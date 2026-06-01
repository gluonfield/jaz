package acp

type CompletionMode int

const (
	CompletionInline CompletionMode = iota
	CompletionAsync
)

func (m CompletionMode) propagates() bool {
	return m == CompletionAsync
}
