package ytdlp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("url") {
		case "https://www.youtube.com/watch?v=jNQXAC9IVRw":
			w.Write([]byte(`{"title":" Me at the zoo ","author_name":"jawed"}`))
		case "https://www.youtube.com/watch?v=private0000":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	old := OEmbedURL
	OEmbedURL = srv.URL
	defer func() { OEmbedURL = old }()

	ctx := context.Background()
	if got, err := Title(ctx, "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil || got != "Me at the zoo" {
		t.Errorf("title: %q %v", got, err)
	}
	if _, err := Title(ctx, "https://www.youtube.com/watch?v=aaaaaaaaaaa"); !errors.Is(err, ErrNoVideo) {
		t.Errorf("missing video: %v", err)
	}
	if _, err := Title(ctx, "https://www.youtube.com/watch?v=private0000"); err == nil || errors.Is(err, ErrNoVideo) {
		t.Errorf("private video must not count as missing: %v", err)
	}
}
