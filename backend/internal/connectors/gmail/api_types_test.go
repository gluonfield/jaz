package gmail

import "testing"

func TestAttachmentsDropsInlineImagesButKeepsRealImageAttachments(t *testing.T) {
	got := attachments(messagePart{Parts: []messagePart{
		{
			MIMEType: "image/png",
			Filename: "image001.png",
			Headers:  []messageHeader{{Name: "Content-ID", Value: "<image001>"}},
			Body:     messageBody{AttachmentID: "inline_1", Size: 123},
		},
		{
			MIMEType: "image/jpeg",
			Filename: "photo.jpg",
			Headers:  []messageHeader{{Name: "Content-Disposition", Value: `attachment; filename="photo.jpg"`}},
			Body:     messageBody{AttachmentID: "photo_1", Size: 456},
		},
		{
			MIMEType: "application/pdf",
			Filename: "plan.pdf",
			Body:     messageBody{AttachmentID: "pdf_1", Size: 789},
		},
	}})

	if len(got) != 2 {
		t.Fatalf("attachments = %#v", got)
	}
	if got[0].ID != "photo_1" || got[0].Inline {
		t.Fatalf("image attachment = %#v", got[0])
	}
	if got[1].ID != "pdf_1" || got[1].Inline {
		t.Fatalf("pdf attachment = %#v", got[1])
	}
}
