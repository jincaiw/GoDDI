package handler

import (
	"encoding/json"
	"errors"
	"io"
)

type dnssecEnableRequest struct {
	Algorithm string `json:"algorithm"`
}

// decodeDNSSECEnableRequest accepts an empty body because algorithm is optional.
func decodeDNSSECEnableRequest(r io.Reader) (dnssecEnableRequest, error) {
	var req dnssecEnableRequest
	dec := json.NewDecoder(r)
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return req, nil
		}
		return req, err
	}
	return req, nil
}
