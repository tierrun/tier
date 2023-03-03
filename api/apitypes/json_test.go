package apitypes

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPhaseResponseJSON(t *testing.T) {
	cases := []struct {
		pr   PhaseResponse
		want string
	}{
		{
			pr:   PhaseResponse{},
			want: `{"effective":"0001-01-01T00:00:00Z"}`,
		},
		{
			pr: PhaseResponse{
				End: time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			want: `{"effective":"0001-01-01T00:00:00Z","end":"2018-01-01T00:00:00Z"}`,
		},
	}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			data, err := json.Marshal(tt.pr)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}
