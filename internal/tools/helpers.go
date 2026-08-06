package tools

import "encoding/json"

func jsonMarshalImpl(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
