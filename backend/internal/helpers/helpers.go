package helpers

import "encoding/json"

func DecodeMap[T any](inputs map[string]any) (T, error) {
	var out T
	data, err := json.Marshal(inputs)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}
