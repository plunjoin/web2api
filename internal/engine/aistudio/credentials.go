package aistudio

import "encoding/json"

// Credentials 引擎B（AI Studio）账号凭据。
type Credentials struct {
	StorageState json.RawMessage `json:"storage_state"`
	Locale       string          `json:"locale,omitempty"`
	Timezone     string          `json:"timezone,omitempty"`
	Proxy        string          `json:"proxy,omitempty"`
}
