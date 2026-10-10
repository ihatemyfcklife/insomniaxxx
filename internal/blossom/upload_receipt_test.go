package blossom

import "testing"

func TestUploadReceiptMustProveMatchingSHA(t *testing.T) {
    good := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    for _, body := range [][]byte{nil, []byte("{}"), []byte("{invalid"), []byte("{\"sha256\":\"wrong\"}")} {
        if err:=validateDescriptor(body,good); err == nil {
            t.Fatalf("invalid receipt accepted: %q",body)
        }
    }
    if err:=validateDescriptor([]byte("{\"sha256\":\""+good+"\"}"),good); err != nil {t.Fatal(err)}
}
