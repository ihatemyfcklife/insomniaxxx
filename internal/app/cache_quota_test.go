package app

import (
 "context"
 "os"
 "path/filepath"
 "testing"
 "time"

 "anonblossom/internal/cryptbox"
)

func TestPlainCacheHasGlobalEntryQuotaAndDeletesEvictedFiles(t *testing.T) {
 c := newDownloadCache(10*time.Minute)
 defer c.Close()
 root := t.TempDir()
 var firstPath string
 for i := 0; i < maxPlainCacheEntries+2; i++ {
  key := string(rune('A'+i))
  path := filepath.Join(root, key+".txt")
  if i==0 { firstPath = path }
  f, _, err := c.GetOrDecrypt(context.Background(),key,func()(string,cryptbox.FileMeta,error){
   if err:=os.WriteFile(path,[]byte("private data"),0600);err!=nil {return "",cryptbox.FileMeta{},err}
   return path,cryptbox.FileMeta{Name:key,Size:12},nil
  })
  if err != nil { t.Fatal(err) }
  f.Close()
  time.Sleep(time.Millisecond)
 }
 c.mu.Lock()
 size,count := c.usedBytes,len(c.entries)
 c.mu.Unlock()
 if count>maxPlainCacheEntries || size>maxPlainCacheBytes {
  t.Fatalf("cache budget exceeded: %d entries, %d bytes",count,size)
 }
 if _,err:=os.Stat(firstPath); !os.IsNotExist(err) {
  t.Fatalf("evicted plaintext persisted on disk: %v",err)
 }
}
