package stripe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kr.dev/diff"
)

func TestList(t *testing.T) {
	t.Run("stops_at_has_more_false", func(t *testing.T) {
		testList(t, `
			{"has_more": true, "data": ["1","2"]}
			{"has_more": true, "data": ["3"]}
			{"has_more": false}
		`,
			[]string{"", "2", "3"},
			[]string{"1", "2", "3"},
		)
	})
	t.Run("stops_at_has_more_true_no_data", func(t *testing.T) {
		testList(t, `
			{"has_more": true, "data": ["1","2"]}
			{"has_more": true, "data": ["3"]}
			{"has_more": true}
		`,
			[]string{"", "2", "3"},
			[]string{"1", "2", "3"},
		)
	})
	t.Run("stops_at_has_more_true_no_data_empty_slice", func(t *testing.T) {
		testList(t, `
			{"has_more": true, "data": ["1","2"]}
			{"has_more": true, "data": ["3"]}
			{"has_more": true, "data": []}
		`,
			[]string{"", "2", "3"},
			[]string{"1", "2", "3"},
		)
	})
	t.Run("single_has_more_true_no_data_empty_slice", func(t *testing.T) {
		testList(t, `
			{"has_more": true, "data": []}
		`,
			[]string{""},
			nil,
		)
	})
	t.Run("single_has_more_false_with_data", func(t *testing.T) {
		testList(t, `
			{"has_more": false, "data": ["1"]}
		`,
			[]string{""},
			[]string{"1"},
		)
	})
	t.Run("single_has_no_more_with_data", func(t *testing.T) {
		testList(t, `
			{"has_more": false, "data": ["1"]}
		`,
			[]string{""},
			[]string{"1"},
		)
	})
	t.Run("single_has_no_more_with_no_data", func(t *testing.T) {
		testList(t, `
			{"has_more": false, "data": []}
		`,
			[]string{""},
			nil,
		)
	})
	t.Run("single_has_no_more_with_no_data_supplied", func(t *testing.T) {
		testList(t, `
			{"has_more": false}
		`,
			[]string{""},
			nil,
		)
	})
	t.Run("single_nothing", func(t *testing.T) {
		testList(t, `
			{}
		`,
			[]string{""},
			nil,
		)
	})
}

func testList(t *testing.T, in string, wantOffsets, wantIDs []string) {
	t.Helper()

	d := json.NewDecoder(strings.NewReader(in))

	var gotOffsets []string
	h := func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		t.Logf("url:%s form:%v", r.URL, r.Form)
		gotOffsets = append(gotOffsets, r.Form.Get("starting_after"))
		var j json.RawMessage
		if err := d.Decode(&j); err != nil {
			t.Fatal(err)
		}
		t.Logf("sending: %v", string(j))
		_, err := w.Write(j)
		if err != nil {
			t.Fatal(err)
		}
	}

	s := httptest.NewServer(http.HandlerFunc(h))
	t.Cleanup(s.Close)

	c := &Client{
		APIKey:     "test",
		BaseURL:    s.URL,
		HTTPClient: s.Client(),
		Logf:       t.Logf,
	}

	ctx := context.Background()
	// use POST so that r.ParseForm() works in our handler above
	vv, err := Slurp[ID](ctx, c, "POST", "/test", Form{})
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, v := range vv {
		gotIDs = append(gotIDs, string(v))
	}

	// gotOffsets will be limited to pre empty data
	t.Run("offsets", func(t *testing.T) {
		t.Helper()
		diff.Test(t, t.Errorf, gotOffsets, wantOffsets)
	})
	t.Run("ids", func(t *testing.T) {
		t.Helper()
		diff.Test(t, t.Errorf, gotIDs, wantIDs)
	})
}
