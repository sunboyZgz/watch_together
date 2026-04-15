package transport

import "encoding/json"

func protocolMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}
