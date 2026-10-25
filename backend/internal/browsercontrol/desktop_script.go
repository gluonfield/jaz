package browsercontrol

import "encoding/json"

type scriptOutput struct {
	Status        string          `json:"status"`
	Text          string          `json:"text,omitempty"`
	ImageBase64   string          `json:"image_base64,omitempty"`
	ImageMIMEType string          `json:"image_mime_type,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
}

func (out scriptOutput) output() (ActionOutput, error) {
	image, err := decodeImageBase64(out.ImageBase64)
	if err != nil {
		return ActionOutput{}, err
	}
	return boundActionOutput(ActionOutput{
		Status:        out.Status,
		Text:          out.Text,
		ImageData:     image,
		ImageMIMEType: out.ImageMIMEType,
		Data:          append(json.RawMessage(nil), out.Data...),
	}), nil
}
