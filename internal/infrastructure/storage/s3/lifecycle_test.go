package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/minio/minio-go/v7"
)

// Exercise the real SDK against S3 responses with several failed objects: a
// single error does not fill its buffered listing/result channels.
func TestRemovalFailureReleasesSDKGoroutines(t *testing.T) {
	for _, method := range []string{"prefix", "recording"} {
		t.Run(method, func(t *testing.T) {
			client := removalTestClient(t, true)
			baseline := removalSDKGoroutines()
			removalSnapshot(t, method+"-before")
			for i := 0; i < 12; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				var err error
				if method == "prefix" {
					err = client.RemovePrefix(ctx, "records/fixture/")
				} else {
					err = client.RemoveRecording(ctx, records.Record{UUID: "8d6c8eec-36c3-43da-8114-319ca701635d"})
				}
				cancel()
				if err == nil || !strings.Contains(err.Error(), "denied") {
					t.Fatalf("expected storage deletion failure, got %v", err)
				}
			}
			deadline := time.Now().Add(time.Second)
			for removalSDKGoroutines() > baseline && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			removalSnapshot(t, method+"-after")
			if got := removalSDKGoroutines(); got > baseline {
				t.Fatalf("SDK goroutines retained after 12 failed deletions and cancellation: before=%d after=%d", baseline, got)
			}
		})
	}
}

func TestRemovalSuccessAndCancellation(t *testing.T) {
	client := removalTestClient(t, false)
	if err := client.RemovePrefix(context.Background(), "records/fixture/"); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveRecording(context.Background(), records.Record{UUID: "8d6c8eec-36c3-43da-8114-319ca701635d"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.RemovePrefix(ctx, "records/fixture/"); err == nil {
		t.Fatal("canceled deletion reported success")
	}
}

func TestRemovePrefixPropagatesListingFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("listing failure must not become a delete request: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "<Error><Code>AccessDenied</Code><Message>Listing denied</Message></Error>")
	}))
	defer server.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	api, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{Region: "us-east-1", Transport: transport, MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{minio: api, bucket: "recordings"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.RemovePrefix(ctx, "records/fixture/"); err == nil || !strings.Contains(err.Error(), "Listing denied") {
		t.Fatalf("listing failure was lost: %v", err)
	}
}

func removalTestClient(t *testing.T, fail bool) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.Method {
		case http.MethodGet:
			versions := r.URL.Query().Has("versions")
			root, item := "ListBucketResult", "Contents"
			if versions {
				root, item = "ListVersionsResult", "Version"
			}
			fmt.Fprintf(w, "<%s><Name>recordings</Name><IsTruncated>false</IsTruncated>", root)
			for i := 0; i < 8; i++ {
				fmt.Fprintf(w, "<%s><Key>%s%d.ts</Key><LastModified>2026-10-01T00:00:00Z</LastModified><Size>1</Size><VersionId>v1</VersionId><IsLatest>true</IsLatest></%s>", item, r.URL.Query().Get("prefix"), i, item)
			}
			fmt.Fprintf(w, "</%s>", root)
		case http.MethodPost:
			fmt.Fprint(w, "<DeleteResult>")
			if fail {
				for i := 0; i < 8; i++ {
					fmt.Fprintf(w, "<Error><Key>records/fixture/%d.ts</Key><Code>AccessDenied</Code><Message>Access denied</Message></Error>", i)
				}
			}
			fmt.Fprint(w, "</DeleteResult>")
		case http.MethodDelete:
			if fail {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, "<Error><Code>AccessDenied</Code><Message>Access denied</Message></Error>")
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		default:
			t.Errorf("unexpected S3 request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	t.Cleanup(transport.CloseIdleConnections)
	api, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{
		Region: "us-east-1", Transport: transport, MaxRetries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Client{minio: api, bucket: "recordings"}
}

func removalSDKGoroutines() int {
	var stacks bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
	count := 0
	for _, stack := range strings.Split(stacks.String(), "\n\n") {
		if strings.Contains(stack, "minio-go/v7.(*Client).ListObjects.func") ||
			strings.Contains(stack, "minio-go/v7.(*Client).RemoveObjects.func") ||
			strings.Contains(stack, "minio-go/v7.(*Client).removeObjects(") {
			count++
		}
	}
	return count
}

func removalSnapshot(t *testing.T, phase string) {
	t.Helper()
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	metrics := map[string]any{"phase": phase, "goroutines": runtime.NumGoroutine(), "sdk_goroutines": removalSDKGoroutines(), "heap_alloc": mem.HeapAlloc}
	data, _ := json.MarshalIndent(metrics, "", "  ")
	t.Log(string(data))
	dir := os.Getenv("P0_STORAGE_PROFILE_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, phase+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"goroutine", "heap", "allocs"} {
		file, err := os.Create(filepath.Join(dir, phase+"-"+name+".pprof"))
		if err != nil {
			t.Fatal(err)
		}
		err = pprof.Lookup(name).WriteTo(file, 0)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write profile: %v %v", err, closeErr)
		}
	}
}
