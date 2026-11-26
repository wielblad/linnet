package meta

import "testing"

func TestParseGIFs(t *testing.T) {
	body := []byte(`{"data":{"comet_external_media":{"result_group":[{"results":[` +
		`{"source":{"mime_type":"image/gif","width":480,"height":360,"size_bytes":148232,"url":"https://media.example/a/giphy.gif","title":"A cat","gif_source":"GIPHY"},"result_id":"one","app_fbid":"1"},` +
		`{"source":{"mime_type":"video/mp4","width":480,"height":360,"url":"https://media.example/b.mp4"},"result_id":"two"},` +
		`{"source":{"mime_type":"image/gif","width":0,"height":0,"url":"https://media.example/c.gif"},"result_id":"three"},` +
		`{"source":{"mime_type":"image/gif","width":10,"height":10,"url":"http://plain.example/d.gif"},"result_id":"four"}` +
		`]}],"result_next_cursor":""}}}` + "\n" + `{"label":"more"}`)
	gifs, err := ParseGIFs(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(gifs) != 1 || gifs[0].ID != "one" || gifs[0].Width != 480 || gifs[0].Title != "A cat" || gifs[0].Source != "GIPHY" {
		t.Errorf("got %+v, want the one usable GIF", gifs)
	}
	if _, err := ParseGIFs([]byte(`{"errors":[{"message":"no such query"}]}`)); err == nil {
		t.Error("an answer that is an error was taken for no GIFs")
	}
}
