package util

import "testing"

func TestParseFFprobeOutputIncludesEmbeddedSubtitles(t *testing.T) {
	out := []byte(`{
		"streams": [
			{"index": 0, "codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080, "avg_frame_rate": "24/1"},
			{"index": 1, "codec_type": "audio", "codec_name": "aac", "sample_rate": "48000", "channels": 2},
			{"index": 2, "codec_type": "subtitle", "codec_name": "ass", "tags": {"language": "chi", "title": "简体中文"}, "disposition": {"default": 1, "forced": 0}},
			{"index": 3, "codec_type": "subtitle", "codec_name": "subrip", "tags": {"language": "eng"}, "disposition": {"default": 0, "forced": 1}}
		],
		"format": {"duration": "120.5", "size": "1024", "format_name": "matroska,webm"}
	}`)

	meta, err := parseFFprobeOutput(out, "movie.mkv")
	if err != nil {
		t.Fatalf("parse ffprobe output: %v", err)
	}
	if len(meta.SubtitleStreams) != 2 {
		t.Fatalf("subtitle streams = %#v, want two", meta.SubtitleStreams)
	}
	first := meta.SubtitleStreams[0]
	if first.Index != 2 || first.Codec != "ass" || first.Language != "chi" || first.Title != "简体中文" || !first.Default || first.Forced {
		t.Fatalf("first subtitle = %#v", first)
	}
	second := meta.SubtitleStreams[1]
	if second.Index != 3 || second.Codec != "subrip" || second.Language != "eng" || second.Default || !second.Forced {
		t.Fatalf("second subtitle = %#v", second)
	}
}
