package command

import "encoding/json"

// isErrorResponse checks whether raw JSON-RPC response data
// contains an ErrorResponse (has a non-empty "code" field)
// rather than a NotificationResponse. Used by both ask and
// dispatch to discriminate on the .response subject.
func isErrorResponse(data []byte) bool {
	var probe struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.Code != ""
}
