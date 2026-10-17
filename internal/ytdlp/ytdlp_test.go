package ytdlp

import (
	"errors"
	"slices"
	"testing"
)

func TestParseURL(t *testing.T) {
	const id = "jNQXAC9IVRw"
	const canonical = "https://www.youtube.com/watch?v=" + id
	ok := []string{
		"https://www.youtube.com/watch?v=" + id,
		"https://www.youtube.com/watch?v=" + id + "&t=754s&list=PL123",
		"http://youtube.com/watch?feature=share&v=" + id,
		"https://m.youtube.com/watch?v=" + id,
		"https://music.youtube.com/watch?v=" + id,
		"https://youtu.be/" + id,
		"https://youtu.be/" + id + "?si=abcDEF",
		"youtu.be/" + id,
		"www.youtube.com/watch?v=" + id,
		"https://www.youtube.com/shorts/" + id,
		"https://www.youtube.com/live/" + id + "?feature=share",
		"https://www.youtube.com/embed/" + id,
		"  https://youtu.be/" + id + "  ",
	}
	for _, in := range ok {
		gotID, gotURL, err := ParseURL(in)
		if err != nil || gotID != id || gotURL != canonical {
			t.Errorf("ParseURL(%q) = %q, %q, %v", in, gotID, gotURL, err)
		}
	}
	bad := []string{"", "hello", "https://vimeo.com/123", "https://www.youtube.com/watch?v=short",
		"https://www.youtube.com/channel/UC123", "ftp://youtu.be/" + id, "https://evil.com/youtu.be/" + id}
	for _, in := range bad {
		if _, _, err := ParseURL(in); !errors.Is(err, ErrNotYouTube) {
			t.Errorf("ParseURL(%q) err = %v; want ErrNotYouTube", in, err)
		}
	}
}

func TestResolveArgs(t *testing.T) {
	got := ResolveArgs("https://www.youtube.com/watch?v=x")
	want := []string{"-J", "--no-playlist", "--no-warnings", "--encoding", "utf-8", "-f", FormatSelector, "-S", "vcodec:h264", "https://www.youtube.com/watch?v=x"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestClientArgsPointAtDeno(t *testing.T) {
	url := "https://www.youtube.com/watch?v=x"
	if got := (Client{}).Args(url); !slices.Equal(got, ResolveArgs(url)) {
		t.Errorf("no deno: got %v", got)
	}
	got := Client{Deno: "/app/tools/deno"}.Args(url)
	if !slices.Equal(got[:2], []string{"--js-runtimes", "deno:/app/tools/deno"}) || !slices.Equal(got[2:], ResolveArgs(url)) {
		t.Errorf("with deno: got %v", got)
	}
}

func TestParseInfoSplitStreams(t *testing.T) {
	data := []byte(`{"id":"abc","title":"Fun Game","duration":3600.5,"width":1920,"height":1080,"live_status":"was_live",
	 "requested_formats":[
	   {"url":"https://v","vcodec":"avc1.640028","acodec":"none","http_headers":{"User-Agent":"UA"}},
	   {"url":"https://a","vcodec":"none","acodec":"mp4a.40.2","http_headers":{"User-Agent":"UA"}}]}`)
	info, err := ParseInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "abc" || info.Title != "Fun Game" || info.Duration != 3600.5 || info.Width != 1920 || info.Height != 1080 {
		t.Errorf("meta = %+v", info)
	}
	if info.Video.URL != "https://v" || info.Video.Headers["User-Agent"] != "UA" || info.Audio == nil || info.Audio.URL != "https://a" {
		t.Errorf("streams = %+v / %+v", info.Video, info.Audio)
	}
}

func TestParseInfoMuxedStream(t *testing.T) {
	info, err := ParseInfo([]byte(`{"id":"abc","title":"t","duration":10,"url":"https://muxed","http_headers":{"A":"b"}}`))
	if err != nil || info.Video.URL != "https://muxed" || info.Audio != nil {
		t.Errorf("got %+v, %v", info, err)
	}
}

func TestParseInfoRejectsLiveAndEmpty(t *testing.T) {
	for _, status := range []string{"is_live", "is_upcoming"} {
		_, err := ParseInfo([]byte(`{"id":"abc","url":"https://x","live_status":"` + status + `"}`))
		if !errors.Is(err, ErrLive) {
			t.Errorf("%s: err = %v; want ErrLive", status, err)
		}
	}
	if _, err := ParseInfo([]byte(`{"id":"abc"}`)); err == nil {
		t.Error("no stream must be an error")
	}
	if _, err := ParseInfo([]byte(`not json`)); err == nil {
		t.Error("bad json must be an error")
	}
}
