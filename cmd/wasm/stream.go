//go:build js && wasm

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"syscall/js"

	"github.com/ihatemyfcklife/badcrypt"

	"anonblossom/internal/cryptbox"
)

const browserStreamChunk = 1 << 20

var (
	activeStreamID int
	activeStream   *cryptbox.StreamEncryptor
	nextHashID     int
	activeHashes   = make(map[int]hash.Hash)
)

func streamEncryptStart(_ js.Value, args []js.Value) any {
	if len(args) < 3 || activeStream != nil {
		return errorObj("streaming encryption is busy or missing metadata")
	}
	size := args[2].Int()
	if size < 0 || size > 64<<20 {
		return errorObj("invalid streaming file size")
	}
	meta := cryptbox.FileMeta{Name: args[0].String(), Type: args[1].String(), Size: int64(size)}
	key, err := cryptbox.NewShareKey()
	if err != nil {
		return errorObj("could not create file encryption key")
	}
	defer badcrypt.Zeroize(key)
	stream, err := cryptbox.NewStreamEncryptor(key, meta, browserStreamChunk)
	if err != nil {
		return errorObj(err.Error())
	}
	activeStreamID++
	activeStream = stream

	header := js.Global().Get("Uint8Array").New(16)
	js.CopyBytesToJS(header, stream.Header())
	result := js.Global().Get("Object").New()
	result.Set("id", activeStreamID)
	result.Set("key", cryptbox.ShareKeyText(key))
	result.Set("header", header)
	return result
}

func streamEncryptPush(_ js.Value, args []js.Value) any {
	if len(args) < 2 || activeStream == nil || args[0].Int() != activeStreamID {
		return errorObj("invalid streaming encryption handle")
	}
	inputSize := args[1].Get("length").Int()
	if inputSize < 0 || inputSize > browserStreamChunk {
		return errorObj("invalid streaming plaintext chunk")
	}
	buf := make([]byte, inputSize)
	js.CopyBytesToGo(buf, args[1])
	defer badcrypt.Zeroize(buf)

	packets := js.Global().Get("Array").New(0)
	err := activeStream.Write(buf, func(packet []byte) error {
		dst := js.Global().Get("Uint8Array").New(len(packet))
		js.CopyBytesToJS(dst, packet)
		packets.Call("push", dst)
		return nil
	})
	if err != nil {
		activeStream.Close()
		activeStream = nil
		return errorObj(err.Error())
	}
	return packets
}

func streamEncryptFinish(_ js.Value, args []js.Value) any {
	if len(args) < 1 || activeStream == nil || args[0].Int() != activeStreamID {
		return errorObj("invalid streaming encryption handle")
	}
	stream := activeStream
	activeStream = nil
	defer stream.Close()
	packets := js.Global().Get("Array").New(0)
	digest, err := stream.Finish(func(packet []byte) error {
		dst := js.Global().Get("Uint8Array").New(len(packet))
		js.CopyBytesToJS(dst, packet)
		packets.Call("push", dst)
		return nil
	})
	if err != nil {
		return errorObj(err.Error())
	}
	result := js.Global().Get("Object").New()
	result.Set("chunks", packets)
	result.Set("hash", digest)
	return result
}

func streamEncryptAbort(_ js.Value, args []js.Value) any {
	if len(args) > 0 && activeStream != nil && args[0].Int() == activeStreamID {
		activeStream.Close()
		activeStream = nil
	}
	return true
}

// Incremental hashing lets browser Blob uploads be validated using bounded
// file slices instead of reading the entire ciphertext into JS/Wasm memory.
func incrementalHashBegin(_ js.Value, _ []js.Value) any {
	if len(activeHashes) >= 2 {
		return errorObj("too many active hashing sessions")
	}
	nextHashID++
	activeHashes[nextHashID] = sha256.New()
	return nextHashID
}

func incrementalHashPush(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return errorObj("missing hashing arguments")
	}
	h := activeHashes[args[0].Int()]
	if h == nil {
		return errorObj("invalid hashing handle")
	}
	n := args[1].Get("length").Int()
	if n < 0 || n > browserStreamChunk {
		return errorObj("invalid hashing chunk size")
	}
	buf := make([]byte, n)
	js.CopyBytesToGo(buf, args[1])
	_, _ = h.Write(buf)
	badcrypt.Zeroize(buf)
	return true
}

func incrementalHashEnd(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorObj("missing hashing handle")
	}
	id := args[0].Int()
	h := activeHashes[id]
	if h == nil {
		return errorObj("invalid hashing handle")
	}
	delete(activeHashes, id)
	return hex.EncodeToString(h.Sum(nil))
}

func incrementalHashAbort(_ js.Value, args []js.Value) any {
	if len(args) > 0 {
		delete(activeHashes, args[0].Int())
	}
	return true
}
