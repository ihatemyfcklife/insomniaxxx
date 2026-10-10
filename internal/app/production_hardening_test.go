package app

import (
    "bytes"
    "io"
    "testing"
)

type alreadyVerified struct { bytes []byte }
func (v alreadyVerified) VerifiedBytes() []byte { return v.bytes }
func (v alreadyVerified) Read(_ []byte) (int,error) { panic("verified data must not be copied") }

func TestVerifiedBlobDoesNotAllocateSecondBuffer(t *testing.T) {
    input := bytes.Repeat([]byte{0x42}, 4096)
    got, err := readLimitedBlob(alreadyVerified{bytes:input}, 4096)
    if err != nil { t.Fatal(err) }
    if &got[0] != &input[0] { t.Fatal("verified blob was copied") }
    if _, err := readLimitedBlob(alreadyVerified{bytes:input}, 4095); err == nil {
        t.Fatal("oversized verified blob was accepted")
    }
    if _, err := readLimitedBlob(io.NopCloser(bytes.NewReader(input)), 4095); err == nil {
        t.Fatal("oversized streaming blob was accepted")
    }
}
func TestInlinePreviewTypesAreExplicitlyAllowlisted(t *testing.T) {
    for _, typ := range []string{"text/html","image/svg+xml","application/pdf","application/xhtml+xml","application/xml","text/plain","image/pngx"} {
        if safeInlineType(typ,"") { t.Fatalf("unsafe inline MIME accepted: %q",typ) }
    }
    for _, typ := range []string{"image/png","image/jpeg","video/mp4","audio/mpeg"} {
        if !safeInlineType(typ,"") { t.Fatalf("safe media MIME rejected: %q",typ) }
    }
    if safeInlineType("image/png","video/") { t.Fatal("wrong preview category") }
}
