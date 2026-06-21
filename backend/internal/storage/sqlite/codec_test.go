package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

// Every block type the app stores must pass the codec allowlist. A type added
// to storage but not to validateBlocks once made AppendUserMessage fail
// silently (the "quote" block). A new BlockType<X> constant must appear in
// both validateBlocks and this table.
func TestMarshalBlocksAcceptsEveryKnownBlockType(t *testing.T) {
	blocks := map[string]storage.Block{
		storage.BlockTypeText:       {Type: storage.BlockTypeText, Text: "hi"},
		storage.BlockTypeReasoning:  {Type: storage.BlockTypeReasoning, Text: "thinking"},
		storage.BlockTypeQuote:      {Type: storage.BlockTypeQuote, Text: "selected"},
		storage.BlockTypeTool:       {Type: storage.BlockTypeTool, ID: "t1", Name: "read"},
		storage.BlockTypeAttachment: {Type: storage.BlockTypeAttachment, ID: "a1", Name: "f.png", URI: "file:///f.png"},
	}
	for name, block := range blocks {
		if _, err := marshalBlocks([]storage.Block{block}); err != nil {
			t.Errorf("marshalBlocks rejected known block type %q: %v", name, err)
		}
	}
}
