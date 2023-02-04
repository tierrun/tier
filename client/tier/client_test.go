package tier

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"kr.dev/diff"
)

func TestUserPassword(t *testing.T) {
	var mu sync.Mutex
	var got []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, _, _ := r.BasicAuth()
		mu.Lock()
		got = append(got, key)
		mu.Unlock()
		io.WriteString(w, "{}")
	}))

	t.Setenv("TIER_BASE_URL", s.URL)
	t.Setenv("TIER_API_KEY", "testkey")

	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Pull(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"testkey"}
	diff.Test(t, t.Errorf, got, want)
}
