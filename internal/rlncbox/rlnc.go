package rlncbox

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ihatemyfcklife/badrlnc"

	"anonblossom/internal/cryptbox"
)

const (
	// Threshold déclenche le partitionnement RLNC pour les fichiers > 2 Mo.
	Threshold = 2 << 20

	// Ratios par défaut
	DefaultK = 4
	DefaultN = 6

	shardMagic      = "ABSH"
	shardHeaderSize = 40 // 4 magic + 1 ver + 1 flags + 2 res + 32 mask (4x uint64)
)

var (
	ErrInsufficientShards = errors.New("rlnc: insufficient shards available to decode")
	ErrLinearlyDependent  = errors.New("rlnc: received shards are linearly dependent")
	ErrCorruptShard       = errors.New("rlnc: corrupt shard payload")
)

// Packet est un alias pour badrlnc.Shard
type Packet = badrlnc.Shard

// Manifest décrit un fichier distribué sous forme de shards RLNC.
type Manifest struct {
	Version       int               `json:"v"`                  // 1
	FileSize      int64             `json:"size"`               // taille totale du clair
	EncryptedSize int64             `json:"enc_size,omitempty"` // taille totale du flux chiffré avant découpage
	ChunkSize     int               `json:"chunk_size"`         // taille unitaire d'un bloc/shard
	K             int               `json:"k"`                  // nombre minimum de shards nécessaires
	N             int               `json:"n"`                  // nombre total de shards émis
	FileMeta      cryptbox.FileMeta `json:"meta"`               // nom, type MIME, etc.
	ShardHashes   []string          `json:"shards"`             // SHA-256 de chaque shard chiffré
	Relays        []string          `json:"relays"`             // URLs des relais utilisés
}

// CalculateKN détermine les paramètres K et N adaptés à la taille du fichier.
// Pour les fichiers de 2 à 4 Mo, K=4, N=6 (shards de ~512 Ko à 1 Mo).
// Pour les fichiers plus gros, shards d'environ 1 Mo max.
func CalculateKN(size int64) (k, n int) {
	if size <= Threshold {
		return 1, 1
	}
	targetChunk := int64(1 << 20) // 1 MiB
	k = int((size + targetChunk - 1) / targetChunk)
	if k < DefaultK {
		k = DefaultK
	}
	if k > 64 {
		k = 64
	}
	n = k + (k / 2)
	if n < DefaultN {
		n = DefaultN
	}
	return k, n
}

// EncodeFile découpe et code un bloc de données en N shards RLNC avec badrlnc.
func EncodeFile(data []byte, k, n int) ([]badrlnc.Shard, error) {
	if k <= 0 || n < k || k > 256 {
		return nil, fmt.Errorf("rlnc: invalid k=%d, n=%d", k, n)
	}
	if len(data) == 0 {
		return nil, errors.New("rlnc: data cannot be empty")
	}

	chunkSize := (len(data) + k - 1) / k
	paddedLen := chunkSize * k
	padded := make([]byte, paddedLen)
	copy(padded, data)

	shards := make([]badrlnc.Shard, n)

	// 1. Shards systématiques (0 .. k-1)
	for i := 0; i < k; i++ {
		chunk := make([]byte, chunkSize)
		copy(chunk, padded[i*chunkSize:(i+1)*chunkSize])
		shards[i] = badrlnc.Shard{
			BaseSeq:  0,
			Mask:     badrlnc.NewBitset256(i),
			Data:     chunk,
			IsParity: false,
		}
	}

	// 2. Shards de parité (k .. n-1)
	// Pour k=4, n=6, sélection de masques optimaux sur GF(2)
	prng := uint64(0x9e3779b97f4a7c15)
	nextRand := func() uint64 {
		prng += 0xbf58476d1ce4e5b9
		z := prng
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		return z ^ (z >> 31)
	}

	for j := k; j < n; j++ {
		var mask badrlnc.Bitset256
		if k == 4 && j == 4 {
			// Masque de parité combinant {0, 1, 2}
			mask = badrlnc.NewBitset256(0, 1, 2)
		} else if k == 4 && j == 5 {
			// Masque de parité combinant {0, 1, 3}
			mask = badrlnc.NewBitset256(0, 1, 3)
		} else if k == 4 && j == 6 {
			mask = badrlnc.NewBitset256(0, 2, 3)
		} else if k == 4 && j == 7 {
			mask = badrlnc.NewBitset256(1, 2, 3)
		} else {
			// Tirage pseudo-aléatoire innovant
			for attempts := 0; attempts < 1000; attempts++ {
				m := badrlnc.Bitset256{}
				r := nextRand()
				for b := 0; b < k; b++ {
					if (r & (1 << (b % 64))) != 0 {
						m.SetBit(b)
					}
				}
				if m.Weight() >= 2 && m.Weight() <= k {
					mask = m
					break
				}
			}
			if mask.IsZero() {
				// Fallback : combine tous les blocs
				for b := 0; b < k; b++ {
					mask.SetBit(b)
				}
			}
		}

		parityData := make([]byte, chunkSize)
		for b := 0; b < k; b++ {
			if mask.TestBit(b) {
				badrlnc.XORBytes(parityData, shards[b].Data, chunkSize)
			}
		}

		shards[j] = badrlnc.Shard{
			BaseSeq:  0,
			Mask:     mask,
			Data:     parityData,
			IsParity: true,
		}
	}

	return shards, nil
}

// DecodeFile reconstruit le fichier original à partir de shards linéairement indépendants
// via élimination de Gauss-Jordan sur GF(2).
func DecodeFile(packets []badrlnc.Shard, k, targetSize int) ([]byte, error) {
	if len(packets) < k {
		return nil, ErrInsufficientShards
	}
	if k <= 0 {
		return nil, errors.New("rlnc: invalid k")
	}

	type row struct {
		mask badrlnc.Bitset256
		data []byte
	}

	rows := make([]row, 0, len(packets))
	chunkSize := 0
	for _, p := range packets {
		if len(p.Data) == 0 {
			continue
		}
		if chunkSize == 0 {
			chunkSize = len(p.Data)
		} else if len(p.Data) != chunkSize {
			return nil, ErrCorruptShard
		}
		d := make([]byte, len(p.Data))
		copy(d, p.Data)
		rows = append(rows, row{
			mask: p.Mask,
			data: d,
		})
	}

	if len(rows) < k {
		return nil, ErrInsufficientShards
	}

	// Élimination de Gauss-Jordan (RREF)
	m := len(rows)
	solved := 0
	for col := 0; col < k; col++ {
		pivotIdx := -1
		for r := solved; r < m; r++ {
			if rows[r].mask.TestBit(col) {
				pivotIdx = r
				break
			}
		}
		if pivotIdx == -1 {
			continue // Pas de pivot trouvé pour cette colonne
		}

		// Placer le pivot à la ligne `solved`
		rows[solved], rows[pivotIdx] = rows[pivotIdx], rows[solved]
		pivotRow := rows[solved]

		// Éliminer `col` de toutes les autres lignes
		for r := 0; r < m; r++ {
			if r != solved && rows[r].mask.TestBit(col) {
				rows[r].mask.XOR(pivotRow.mask)
				badrlnc.XORBytes(rows[r].data, pivotRow.data, chunkSize)
			}
		}
		solved++
	}

	// Vérifier si toutes les variables 0..k-1 sont résolues (poids 1 et bit i à 1)
	resolvedChunks := make([][]byte, k)
	for col := 0; col < k; col++ {
		found := false
		for _, r := range rows {
			if r.mask.Weight() == 1 && r.mask.TestBit(col) {
				resolvedChunks[col] = r.data
				found = true
				break
			}
		}
		if !found {
			return nil, ErrLinearlyDependent
		}
	}

	// Reconstitution du buffer
	totalLen := 0
	for _, chunk := range resolvedChunks {
		totalLen += len(chunk)
	}
	out := make([]byte, 0, totalLen)
	for _, chunk := range resolvedChunks {
		out = append(out, chunk...)
	}

	if targetSize > 0 && len(out) > targetSize {
		out = out[:targetSize]
	}
	return out, nil
}

// IsInnovative teste si un nouveau masque augmente le rang de l'espace vectoriel
// formé par une liste de masques existants sur GF(2).
func IsInnovative(existing []badrlnc.Bitset256, newMask badrlnc.Bitset256) bool {
	if newMask.IsZero() {
		return false
	}

	var pivots [256]badrlnc.Bitset256
	var hasPivot [256]bool

	insert := func(v badrlnc.Bitset256) bool {
		for {
			tz := v.TrailingZeros()
			if tz < 0 {
				return false // v est nul sur GF(2)
			}
			if !hasPivot[tz] {
				pivots[tz] = v
				hasPivot[tz] = true
				return true
			}
			v.XOR(pivots[tz])
		}
	}

	for _, m := range existing {
		insert(m)
	}

	return insert(newMask)
}

// SerializeShard sérialise un Shard en flux binaire avec l'en-tête RLNC de 40 octets.
func SerializeShard(shard badrlnc.Shard) []byte {
	buf := make([]byte, shardHeaderSize+len(shard.Data))
	copy(buf[:4], shardMagic)
	buf[4] = 1 // Version 1
	if shard.IsParity {
		buf[5] = 1 // Flag parity
	}
	binary.BigEndian.PutUint64(buf[8:16], shard.Mask[0])
	binary.BigEndian.PutUint64(buf[16:24], shard.Mask[1])
	binary.BigEndian.PutUint64(buf[24:32], shard.Mask[2])
	binary.BigEndian.PutUint64(buf[32:40], shard.Mask[3])
	copy(buf[shardHeaderSize:], shard.Data)
	return buf
}

// DeserializeShard désérialise un Shard depuis son flux binaire.
func DeserializeShard(b []byte) (badrlnc.Shard, error) {
	if len(b) < shardHeaderSize || string(b[:4]) != shardMagic {
		return badrlnc.Shard{}, ErrCorruptShard
	}
	if b[4] != 1 {
		return badrlnc.Shard{}, errors.New("rlnc: unsupported version")
	}
	isParity := (b[5] & 1) != 0
	var mask badrlnc.Bitset256
	mask[0] = binary.BigEndian.Uint64(b[8:16])
	mask[1] = binary.BigEndian.Uint64(b[16:24])
	mask[2] = binary.BigEndian.Uint64(b[24:32])
	mask[3] = binary.BigEndian.Uint64(b[32:40])

	data := make([]byte, len(b)-shardHeaderSize)
	copy(data, b[shardHeaderSize:])
	return badrlnc.Shard{
		BaseSeq:  0,
		Mask:     mask,
		Data:     data,
		IsParity: isParity,
	}, nil
}
