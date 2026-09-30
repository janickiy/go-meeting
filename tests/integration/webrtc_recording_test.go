package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
)

// This opt-in test creates a synthetic recording in the local running stack.
// It never accesses a browser, camera, microphone, or an existing recording.
func TestSyntheticWebRTCRecording(t *testing.T) {
	base, fixture := os.Getenv("RECORDER_TEST_URL"), os.Getenv("RECORDER_TEST_IVF")
	if base == "" || fixture == "" {
		t.Skip("set RECORDER_TEST_URL and RECORDER_TEST_IVF (VP8 IVF at 25 fps)")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("integration test only supports a local API")
	}
	file, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, header, err := ivfreader.NewWith(file)
	if err != nil || header.FourCC != "VP80" {
		t.Fatalf("expected a VP8 IVF fixture: %v", err)
	}
	api := &localAPI{base: strings.TrimRight(base, "/"), client: &http.Client{Timeout: 10 * time.Second}}
	var start records.StartResponse
	api.json(t, "POST", "/api/v1/records/start", records.StartRequest{ConferenceID: uuid.NewString(), QualityMode: "auto", SegmentDurationSec: 2}, &start)
	t.Logf("synthetic record: %s", start.RecordID)
	stopped := false
	defer func() {
		if !stopped {
			api.json(t, "POST", "/api/v1/records/end", records.EndRequest{RecordID: start.RecordID, Reason: "synthetic_test_cleanup"}, nil)
		}
	}()
	for deadline := time.Now().Add(5 * time.Second); ; {
		card := api.record(t, start.RecordID)
		if len(card.Events) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not prepare the recording")
		}
		time.Sleep(100 * time.Millisecond)
	}
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	states := make(chan webrtc.PeerConnectionState, 8)
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		select {
		case states <- state:
		default:
		}
	})
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "synthetic-video", "synthetic-stream")
	if err != nil {
		t.Fatal(err)
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(buf); err != nil {
				return
			}
		}
	}()
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		t.Fatal("local ICE gathering timed out")
	}
	var answer webrtc.SessionDescription
	api.json(t, "POST", start.WebRTC.OfferURL, pc.LocalDescription(), &answer)
	for _, line := range strings.Split(answer.SDP, "\n") {
		if strings.HasPrefix(line, "a=candidate:") {
			fields := strings.Fields(line)
			if len(fields) >= 6 {
				t.Logf("worker ICE: %s %s:%s", fields[2], fields[4], fields[5])
				if ip := net.ParseIP(fields[4]); ip != nil && ip.IsLoopback() {
					t.Fatal("worker advertises a loopback ICE address, incompatible with Firefox defaults")
				}
			}
		}
	}
	if err := pc.SetRemoteDescription(answer); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
connected:
	for {
		select {
		case state := <-states:
			t.Logf("WebRTC: %s", state)
			if state == webrtc.PeerConnectionStateConnected {
				break connected
			}
			if state == webrtc.PeerConnectionStateFailed {
				t.Fatal("WebRTC connection failed")
			}
		case <-timer.C:
			t.Fatal("WebRTC connection timed out")
		}
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	frames := 0
	for {
		frame, _, err := reader.ParseNextFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		<-ticker.C
		if err := track.WriteSample(media.Sample{Data: frame, Duration: 40 * time.Millisecond}); err != nil {
			t.Fatal(err)
		}
		frames++
	}
	t.Logf("sent %d synthetic video frames", frames)
	if frames < 150 {
		t.Fatal("fixture must contain at least 6 seconds of video")
	}
	if card := api.record(t, start.RecordID); card.Status != records.StatusRecording {
		t.Fatalf("expected recording, got %s", card.Status)
	}
	api.json(t, "POST", "/api/v1/records/end", records.EndRequest{RecordID: start.RecordID, Reason: "synthetic_test"}, nil)
	stopped = true
	for deadline := time.Now().Add(60 * time.Second); ; {
		card := api.record(t, start.RecordID)
		if card.Status == records.StatusFailed {
			if card.ErrorMessage != nil {
				t.Fatalf("recording failed: %s", *card.ErrorMessage)
			}
			t.Fatal("recording failed without an error message")
		}
		if card.Status == records.StatusReady {
			if len(card.Files) < 2 {
				t.Fatal("final video or preview is missing")
			}
			for _, artifact := range card.Files {
				resp, err := api.client.Get(artifact.URL)
				if err != nil {
					t.Fatal(err)
				}
				n, readErr := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 || readErr != nil || n == 0 {
					t.Fatalf("artifact %s is unavailable: status=%d bytes=%d err=%v", artifact.FileType, resp.StatusCode, n, readErr)
				}
				t.Logf("artifact %s: %d bytes", artifact.FileType, n)
			}
			t.Log("recording is ready in MinIO")
			var summary struct {
				Items []records.ConferenceRecordSummary `json:"items"`
			}
			api.json(t, "GET", "/api/v1/records/count-by-conference?conferenceIds="+url.QueryEscape(start.ConferenceID)+"&status=ready", nil, &summary)
			if len(summary.Items) != 1 || summary.Items[0].RecordsCount != 1 || len(summary.Items[0].Records) != 1 {
				t.Fatalf("unexpected conference summary: %+v", summary.Items)
			}
			item := summary.Items[0].Records[0]
			if item.RecordID != start.RecordID || item.FinalURL == "" || item.PreviewURL == "" {
				t.Fatalf("summary is missing record or artifact links: %+v", item)
			}
			api.json(t, "POST", "/api/v1/records/end", records.EndRequest{RecordID: start.RecordID, Reason: "duplicate_test_stop"}, nil)
			afterRetry := api.record(t, start.RecordID)
			if afterRetry.Status != records.StatusReady || !afterRetry.UpdatedAt.Equal(card.UpdatedAt) {
				t.Fatalf("duplicate stop changed the ready record: %s", afterRetry.Status)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("finalization timed out in status %s", card.Status)
		}
		time.Sleep(time.Second)
	}
}

type localAPI struct {
	base   string
	client *http.Client
}

func (a *localAPI) record(t *testing.T, id string) records.RecordCard {
	t.Helper()
	var result struct {
		Item records.RecordCard `json:"item"`
	}
	a.json(t, "GET", "/api/v1/records/"+id, nil, &result)
	return result.Item
}

func (a *localAPI) json(t *testing.T, method, path string, payload, result any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, a.base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatal(fmt.Sprintf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, data))
	}
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			t.Fatal(err)
		}
	}
}
