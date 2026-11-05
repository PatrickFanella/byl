package httpapi

import "testing"

func TestUploadContentTypeUsesFileBytes(t *testing.T) {
	if got := detectContentType("image/png", "misleading.png", []byte("\xff\xd8\xffjpeg")); got != "image/jpeg" {
		t.Fatalf("JPEG with misleading metadata detected as %q", got)
	}
	if got := detectContentType("image/png", "active.png", []byte("<html><script>alert(1)</script>")); got != "text/html" {
		t.Fatalf("HTML with misleading metadata detected as %q", got)
	}
}
