package payment

import (
	"encoding/json"
	"time"
)

type Method struct {
	raw  json.RawMessage // hidden so we can add fields to Method later, and replace if neccessary
	info struct {
		ID      string
		Created int
	}
}

func (m *Method) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &m.raw); err != nil {
		return err
	}
	return json.Unmarshal(data, &m.info)
}

func (m Method) MarshalJSON() ([]byte, error) {
	return m.raw.MarshalJSON()
}

func (m Method) Created() time.Time {
	return time.Unix(int64(m.info.Created), 0)
}

func (m Method) ProviderID() string {
	return m.info.ID
}
