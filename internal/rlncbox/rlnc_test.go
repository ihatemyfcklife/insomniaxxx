package rlncbox

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/ihatemyfcklife/badrlnc"
)

func TestRLNC_EncodeDecodeNoLoss(t *testing.T) {
	data := []byte("Hello, RLNC Distributed Blossom World! Testing standard buffer without loss.")
	k, n := 4, 6
	shards, err := EncodeFile(data, k, n)
	if err != nil {
		t.Fatalf("EncodeFile failed: %v", err)
	}
	if len(shards) != n {
		t.Fatalf("expected %d shards, got %d", n, len(shards))
	}

	// Décodage avec les K premiers shards
	recovered, err := DecodeFile(shards[:k], k, len(data))
	if err != nil {
		t.Fatalf("DecodeFile failed: %v", err)
	}
	if !bytes.Equal(recovered, data) {
		t.Fatalf("recovered data mismatch: got %q, want %q", string(recovered), string(data))
	}
}

func TestRLNC_EncodeDecodeWithParity(t *testing.T) {
	size := 3 * 1024 * 1024 // 3 Mo
	data := make([]byte, size)
	_, _ = rand.Read(data)

	k, n := 4, 6
	shards, err := EncodeFile(data, k, n)
	if err != nil {
		t.Fatalf("EncodeFile failed: %v", err)
	}

	// Simulation de perte du shard 0 et du shard 2 : on prend [1, 3, 4, 5]
	subset := []badrlnc.Shard{shards[1], shards[3], shards[4], shards[5]}
	recovered, err := DecodeFile(subset, k, size)
	if err != nil {
		t.Fatalf("DecodeFile with parity failed: %v", err)
	}
	if !bytes.Equal(recovered, data) {
		t.Fatalf("recovered data mismatch with parity")
	}
}

func TestRLNC_Serialization(t *testing.T) {
	orig := badrlnc.Shard{
		BaseSeq:  0,
		Mask:     badrlnc.NewBitset256(1, 3, 5),
		Data:     []byte("test shard payload data"),
		IsParity: true,
	}

	wire := SerializeShard(orig)
	deser, err := DeserializeShard(wire)
	if err != nil {
		t.Fatalf("DeserializeShard failed: %v", err)
	}

	if deser.Mask != orig.Mask {
		t.Errorf("mask mismatch")
	}
	if deser.IsParity != orig.IsParity {
		t.Errorf("isParity mismatch")
	}
	if !bytes.Equal(deser.Data, orig.Data) {
		t.Errorf("data mismatch")
	}
}

func TestRLNC_IsInnovative(t *testing.T) {
	var basis []badrlnc.Bitset256

	m0 := badrlnc.NewBitset256(0)
	if !IsInnovative(basis, m0) {
		t.Errorf("m0 should be innovative")
	}
	basis = append(basis, m0)

	m1 := badrlnc.NewBitset256(1)
	if !IsInnovative(basis, m1) {
		t.Errorf("m1 should be innovative")
	}
	basis = append(basis, m1)

	// Combinaison dépendante m0 ^ m1
	mComb := badrlnc.NewBitset256(0, 1)
	if IsInnovative(basis, mComb) {
		t.Errorf("mComb should NOT be innovative (already spanned by m0 and m1)")
	}

	// Nouveau vecteur m2
	m2 := badrlnc.NewBitset256(2)
	if !IsInnovative(basis, m2) {
		t.Errorf("m2 should be innovative")
	}
}

func TestRLNC_CalculateKN(t *testing.T) {
	k, n := CalculateKN(1024)
	if k != 4 || n != 6 {
		t.Errorf("expected 4, 6 for small file, got %d, %d", k, n)
	}
	for _, size := range []int64{0, 1, 64, 1 << 20, Threshold} {
		k, n := CalculateKN(size)
		if k != 4 || n != 6 {
			t.Errorf("size %d: expected minimum 4+2 shards, got %d, %d", size, k, n)
		}
	}

	k, n = CalculateKN(3 * 1024 * 1024)
	if k != 4 || n != 6 {
		t.Errorf("expected 4, 6 for 3MB file, got %d, %d", k, n)
	}

	k, n = CalculateKN(10 * 1024 * 1024)
	if k < 4 || n < k {
		t.Errorf("unexpected kn for 10MB: %d, %d", k, n)
	}
}
