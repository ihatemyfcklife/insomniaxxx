//go:build js && wasm

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"syscall/js"

	"github.com/ihatemyfcklife/badcrypt"
	"github.com/ihatemyfcklife/badrlnc"

	"anonblossom/internal/cryptbox"
	"anonblossom/internal/qr"
	"anonblossom/internal/rlncbox"
	"anonblossom/internal/uploadauth"
)

func main() {
	c := make(chan struct{}, 0)

	js.Global().Set("wasmUploadAuthorization", js.FuncOf(uploadAuthorization))
	js.Global().Set("wasmEncrypt", js.FuncOf(encryptFile))
	js.Global().Set("wasmEncryptWithKey", js.FuncOf(encryptFileWithKey))
	js.Global().Set("wasmDecrypt", js.FuncOf(decryptBlob))
	js.Global().Set("wasmEncodeRLNC", js.FuncOf(encodeRLNC))
	js.Global().Set("wasmDecodeRLNC", js.FuncOf(decodeRLNC))
	js.Global().Set("wasmIsInnovative", js.FuncOf(isInnovative))
	js.Global().Set("wasmGenerateQR", js.FuncOf(generateQR))
	js.Global().Set("wasmSHA256", js.FuncOf(hashBytes))
	js.Global().Set("wasmEncryptStreamStart", js.FuncOf(streamEncryptStart))
	js.Global().Set("wasmEncryptStreamPush", js.FuncOf(streamEncryptPush))
	js.Global().Set("wasmEncryptStreamFinish", js.FuncOf(streamEncryptFinish))
	js.Global().Set("wasmEncryptStreamAbort", js.FuncOf(streamEncryptAbort))
	js.Global().Set("wasmSHA256Begin", js.FuncOf(incrementalHashBegin))
	js.Global().Set("wasmSHA256Push", js.FuncOf(incrementalHashPush))
	js.Global().Set("wasmSHA256End", js.FuncOf(incrementalHashEnd))
	js.Global().Set("wasmSHA256Abort", js.FuncOf(incrementalHashAbort))

	<-c
}

// encryptFile prend (Uint8Array, filename, mimeType) et renvoie un objet JS
func encryptFile(this js.Value, args []js.Value) any {
	if len(args) < 3 {
		return errorObj("arguments manquants")
	}

	srcBytes := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(srcBytes, args[0])
	filename := args[1].String()
	mimeType := args[2].String()

	meta := cryptbox.FileMeta{
		Name: filename,
		Type: mimeType,
		Size: int64(len(srcBytes)),
	}

	var encryptedBuf bytes.Buffer
	key, err := cryptbox.EncryptShare(bytes.NewReader(srcBytes), &encryptedBuf, meta, 1<<20)
	if err != nil {
		return errorObj(err.Error())
	}
	defer badcrypt.Zeroize(key)

	cipherBytes := encryptedBuf.Bytes()
	hasher := sha256.New()
	hasher.Write(cipherBytes)
	blobHash := hex.EncodeToString(hasher.Sum(nil))

	dst := js.Global().Get("Uint8Array").New(len(cipherBytes))
	js.CopyBytesToJS(dst, cipherBytes)

	res := js.Global().Get("Object").New()
	res.Set("ciphertext", dst)
	res.Set("hash", blobHash)
	res.Set("key", cryptbox.ShareKeyText(key))
	return res
}

// encryptFileWithKey prend (Uint8Array, filename, mimeType, keyText) et chiffre avec la clé spécifiée
func encryptFileWithKey(this js.Value, args []js.Value) any {
	if len(args) < 4 {
		return errorObj("arguments manquants: data, filename, mimeType, keyText")
	}

	srcBytes := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(srcBytes, args[0])
	filename := args[1].String()
	mimeType := args[2].String()
	keyText := args[3].String()

	_, key, ok := cryptbox.ParseDownloadPath("/d/0000000000000000000000000000000000000000000000000000000000000000/" + keyText)
	if !ok {
		return errorObj("clé invalide")
	}
	defer badcrypt.Zeroize(key)

	meta := cryptbox.FileMeta{
		Name: filename,
		Type: mimeType,
		Size: int64(len(srcBytes)),
	}

	var encryptedBuf bytes.Buffer
	err := cryptbox.EncryptShareWithKey(key, bytes.NewReader(srcBytes), &encryptedBuf, meta, 64<<10)
	if err != nil {
		return errorObj(err.Error())
	}

	cipherBytes := encryptedBuf.Bytes()
	hasher := sha256.New()
	hasher.Write(cipherBytes)
	blobHash := hex.EncodeToString(hasher.Sum(nil))

	dst := js.Global().Get("Uint8Array").New(len(cipherBytes))
	js.CopyBytesToJS(dst, cipherBytes)

	res := js.Global().Get("Object").New()
	res.Set("ciphertext", dst)
	res.Set("hash", blobHash)
	res.Set("key", keyText)
	return res
}

// decryptBlob prend (Uint8Array, keyText) et renvoie un Blob/fichier déchiffré
func decryptBlob(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return errorObj("arguments manquants")
	}

	cipherBytes := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(cipherBytes, args[0])
	keyText := args[1].String()

	_, key, ok := cryptbox.ParseDownloadPath("/d/0000000000000000000000000000000000000000000000000000000000000000/" + keyText)
	if !ok {
		return errorObj("clé de déchiffrement invalide")
	}
	defer badcrypt.Zeroize(key)

	var plainBuf bytes.Buffer
	meta, err := cryptbox.DecryptTo(&plainBuf, bytes.NewReader(cipherBytes), int64(len(cipherBytes)), key, 64<<20)
	if err != nil {
		return errorObj("échec déchiffrement: " + err.Error())
	}

	outBytes := plainBuf.Bytes()
	dst := js.Global().Get("Uint8Array").New(len(outBytes))
	js.CopyBytesToJS(dst, outBytes)

	res := js.Global().Get("Object").New()
	res.Set("data", dst)
	res.Set("name", meta.Name)
	res.Set("type", meta.Type)
	res.Set("size", meta.Size)
	return res
}

// encodeRLNC prend (Uint8Array, k, n) et renvoie un Array JS de Uint8Array (les shards sérialisés)
func encodeRLNC(this js.Value, args []js.Value) any {
	if len(args) < 3 {
		return errorObj("arguments manquants: data, k, n")
	}

	srcBytes := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(srcBytes, args[0])
	k := args[1].Int()
	n := args[2].Int()

	shards, err := rlncbox.EncodeFile(srcBytes, k, n)
	if err != nil {
		return errorObj("échec encodage RLNC: " + err.Error())
	}

	resArray := js.Global().Get("Array").New(len(shards))
	for i, s := range shards {
		raw := rlncbox.SerializeShard(s)
		dst := js.Global().Get("Uint8Array").New(len(raw))
		js.CopyBytesToJS(dst, raw)
		resArray.SetIndex(i, dst)
	}
	return resArray
}

// decodeRLNC prend (Array de Uint8Array, k, targetSize) et renvoie un Uint8Array
func decodeRLNC(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return errorObj("arguments manquants: shardsArray, k, [targetSize]")
	}

	shardsArray := args[0]
	k := args[1].Int()
	targetSize := 0
	if len(args) >= 3 && !args[2].IsUndefined() && !args[2].IsNull() {
		targetSize = args[2].Int()
	}

	numShards := shardsArray.Get("length").Int()
	if numShards < k {
		return errorObj("nombre de shards insuffisant")
	}

	shards := make([]badrlnc.Shard, 0, numShards)
	for i := 0; i < numShards; i++ {
		item := shardsArray.Index(i)
		buf := make([]byte, item.Get("length").Int())
		js.CopyBytesToGo(buf, item)

		s, err := rlncbox.DeserializeShard(buf)
		if err != nil {
			return errorObj("shard invalide: " + err.Error())
		}
		shards = append(shards, s)
	}

	decoded, err := rlncbox.DecodeFile(shards, k, targetSize)
	if err != nil {
		return errorObj("échec décodage RLNC: " + err.Error())
	}

	dst := js.Global().Get("Uint8Array").New(len(decoded))
	js.CopyBytesToJS(dst, decoded)
	return dst
}

// isInnovative prend (Array de shards existants, nouveau shard) et renvoie bool
func isInnovative(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return false
	}
	existingArray := args[0]
	newShardVal := args[1]

	newBuf := make([]byte, newShardVal.Get("length").Int())
	js.CopyBytesToGo(newBuf, newShardVal)
	newShard, err := rlncbox.DeserializeShard(newBuf)
	if err != nil {
		return false
	}

	numExisting := existingArray.Get("length").Int()
	basis := make([]badrlnc.Bitset256, 0, numExisting)
	for i := 0; i < numExisting; i++ {
		item := existingArray.Index(i)
		buf := make([]byte, item.Get("length").Int())
		js.CopyBytesToGo(buf, item)
		s, err := rlncbox.DeserializeShard(buf)
		if err == nil {
			basis = append(basis, s.Mask)
		}
	}

	return rlncbox.IsInnovative(basis, newShard.Mask)
}

// Generate the QR image in-browser to keep #fragment secrets off the wire.
func generateQR(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorObj("missing QR value")
	}
	payload := args[0].String()
	if len(payload) == 0 || len(payload) > 4096 {
		return errorObj("invalid QR value")
	}
	png, err := qr.GeneratePNG(payload)
	if err != nil {
		return errorObj("QR generation failed")
	}
	out := js.Global().Get("Uint8Array").New(len(png))
	js.CopyBytesToJS(out, png)
	return out
}

// Hash shard bytes in WASM to verify the manifest before accepting them.
func hashBytes(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorObj("missing data")
	}
	buf := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(buf, args[0])
	hash := sha256.Sum256(buf)
	return hex.EncodeToString(hash[:])
}

func errorObj(msg string) js.Value {
	res := js.Global().Get("Object").New()
	res.Set("error", msg)
	return res
}

// uploadAuthorization signs locally; no file key or signing secret is exported.
func uploadAuthorization(this js.Value, args []js.Value) any {
	if len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		return errorObj("invalid relay authorization arguments")
	}
	token, err := uploadauth.Sign(args[0].String(), args[1].String())
	if err != nil {
		return errorObj("relay authorization failed")
	}
	return token
}
