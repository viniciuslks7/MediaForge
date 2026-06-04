package media

import "testing"

func TestKindRoutingKey(t *testing.T) {
	cases := map[Kind]string{
		KindImage: "image.process",
		KindOCR:   "ocr.extract",
		Kind("x"): "image.process", // unknown falls back to image
	}
	for k, want := range cases {
		if got := k.RoutingKey(); got != want {
			t.Errorf("Kind(%q).RoutingKey() = %q, want %q", k, got, want)
		}
	}
}

func TestKindValid(t *testing.T) {
	if !KindImage.Valid() || !KindOCR.Valid() {
		t.Fatal("image and ocr must be valid kinds")
	}
	if Kind("video").Valid() {
		t.Fatal("video is not a supported kind yet")
	}
}
