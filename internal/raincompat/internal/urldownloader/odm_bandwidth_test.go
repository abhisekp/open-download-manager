package urldownloader

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cenkalti/rain/v2/internal/bandwidth"
	"github.com/cenkalti/rain/v2/internal/bufferpool"
	"github.com/cenkalti/rain/v2/internal/filesection"
	"github.com/cenkalti/rain/v2/internal/piece"
)

func TestWebseedPayloadCapDoesNotConsumeReadTimeout(t *testing.T) {
	payload := bytes.Repeat([]byte{0x4f, 0x44, 0x4d, 0x21}, 32768)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	const size = 65536
	pieces := make([]piece.Piece, 2)
	for i := range pieces {
		pieces[i] = piece.Piece{Index: uint32(i), Length: size, Data: filesection.Piece{{Name: "original.bin", Offset: int64(i * size), Length: size}}}
	}
	downloader := New(server.URL, 0, 2, bandwidth.New(size))
	results := make(chan *PieceResult)
	start := time.Now()
	go downloader.Run(server.Client(), pieces, false, results, bufferpool.New(size), 100*time.Millisecond)
	defer downloader.Close()
	var actual []byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-results:
			if result.Error != nil {
				t.Fatal(result.Error)
			}
			actual = append(actual, result.Buffer.Data...)
			result.Buffer.Release()
		case <-time.After(10 * time.Second):
			t.Fatal("webseed cap stalled")
		}
	}
	if time.Since(start) < 800*time.Millisecond {
		t.Fatal("webseed bypassed shared cap")
	}
	if !bytes.Equal(actual, payload) {
		t.Fatal("webseed cap changed file bytes")
	}
}
