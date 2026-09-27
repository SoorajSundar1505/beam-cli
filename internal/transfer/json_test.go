package transfer

import "encoding/json"

func unmarshalJSON(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
