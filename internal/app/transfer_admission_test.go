package app

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Test admission under a burst of one thousand concurrent transfer requests.
// This is NOT a 1,000 end-to-end file transfer throughput benchmark.
func TestTransferAdmissionBoundedUnderThousandClients(t *testing.T) {
	a := &Server{downloads:make(chan struct{},32),heavyDownloads:make(chan struct{},4)}
	const clients=1000
	start:=make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(clients)
	results:=make(chan int,clients)
	for i:=0;i<clients;i++ {
		go func() {
			defer wg.Done()
			<-start
			w:=httptest.NewRecorder()
			r:=httptest.NewRequest(http.MethodGet,"/d/test?cipher=1",nil)
			_ = r
			if a.acquireDownload(w) {
				defer a.releaseDownload()
				results<-http.StatusOK
			} else {
				results<-w.Code
			}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success,rejected:=0,0
	for status:=range results {
		switch status {
		case http.StatusOK: success++
		case http.StatusServiceUnavailable: rejected++
		default:t.Fatalf("unexpected response status: %d",status)
		}
	}
	if success+rejected!=clients || rejected==0 {
		t.Fatalf("unbounded or lost transfer admissions: %d accepted, %d refused",success,rejected)
	}
	if len(a.downloads)!=0 { t.Fatal("leaked cipher download slot") }
	if len(a.heavyDownloads)!=0 { t.Fatal("leaked plaintext download slot") }
}
