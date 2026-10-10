package rlncbox

import (
    "bytes"
    "testing"

    "github.com/ihatemyfcklife/badrlnc"
)

// Binary XOR coding is not MDS: not every k-of-n subset is independent.
// Lock down the truthful loss matrix and never market it as Reed-Solomon.
func TestFourOfSixLossMatrixIsDocumented(t *testing.T) {
    data:=bytes.Repeat([]byte("deterministic four of six data"),256)
    shards,err:=EncodeFile(data,4,6)
    if err!=nil {t.Fatal(err)}
    lostPairs:=0
    for a:=0;a<6;a++ {
        for b:=a+1;b<6;b++ {
            remaining:=make([]badrlnc.Shard,0,4)
            for i,s:=range shards {if i!=a && i!=b {remaining=append(remaining,s)}}
            got,err:=DecodeFile(remaining,4,len(data))
            if err!=nil {
                lostPairs++
                continue
            }
            if !bytes.Equal(got,data) {t.Fatalf("silent corrupt recovery for lost %d,%d",a,b)}
        }
    }
    if lostPairs!=3 {
        t.Fatalf("four-of-six GF(2) loss matrix changed: got %d unrecoverable pairs, want 3",lostPairs)
    }
}
