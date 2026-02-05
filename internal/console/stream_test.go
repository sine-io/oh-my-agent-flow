package console

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamHandler_ReplaysEventsWithSinceSeq(t *testing.T) {
	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 100, SubscriberBufSize: 16})
	hub.Publish(StreamEvent{RunID: "r1", Type: "process_stdout", Step: "fire", Level: "info", Data: map[string]any{"text": "a"}})
	hub.Publish(StreamEvent{RunID: "r1", Type: "process_stdout", Step: "fire", Level: "info", Data: map[string]any{"text": "b"}})
	hub.Publish(StreamEvent{RunID: "r1", Type: "process_stdout", Step: "fire", Level: "info", Data: map[string]any{"text": "c"}})

	srv := httptest.NewServer(http.HandlerFunc(StreamHandler(hub)))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?runId=r1&sinceSeq=1", nil)
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	ev2 := mustReadNextSSEEvent(t, reader)
	ev3 := mustReadNextSSEEvent(t, reader)
	cancel()

	if ev2.RunID != "r1" || ev2.Seq != 2 {
		t.Fatalf("unexpected event2: runId=%q seq=%d", ev2.RunID, ev2.Seq)
	}
	if ev3.RunID != "r1" || ev3.Seq != 3 {
		t.Fatalf("unexpected event3: runId=%q seq=%d", ev3.RunID, ev3.Seq)
	}
}

func TestStreamHandler_EmitsReplayTruncatedProgressOnce(t *testing.T) {
	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 2, SubscriberBufSize: 16})
	for i := 0; i < 5; i++ {
		hub.Publish(StreamEvent{RunID: "r2", Type: "process_stdout", Step: "fire", Level: "info", Data: map[string]any{"text": "x"}})
	}

	srv := httptest.NewServer(http.HandlerFunc(StreamHandler(hub)))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?runId=r2&sinceSeq=1", nil)
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	ev4 := mustReadNextSSEEvent(t, reader)
	ev5 := mustReadNextSSEEvent(t, reader)
	ev6 := mustReadNextSSEEvent(t, reader)
	cancel()

	if ev4.Seq != 4 || ev5.Seq != 5 || ev6.Seq != 6 {
		t.Fatalf("expected seq 4,5,6 got %d,%d,%d", ev4.Seq, ev5.Seq, ev6.Seq)
	}
	if ev6.Type != "progress" {
		t.Fatalf("expected progress event, got %q", ev6.Type)
	}
	data, ok := ev6.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected progress data object, got %T", ev6.Data)
	}
	if data["phase"] != "error" {
		t.Fatalf("expected phase=error, got %v", data["phase"])
	}
	note, _ := data["note"].(string)
	if !strings.Contains(note, "replay truncated") {
		t.Fatalf("unexpected note: %q", note)
	}
}

func TestStreamHandler_GlobalStream_IsBestEffortAndLive(t *testing.T) {
	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 100, SubscriberBufSize: 16})
	srv := httptest.NewServer(http.HandlerFunc(StreamHandler(hub)))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.Publish(StreamEvent{RunID: "r3", Type: "run_started", Step: "fire", Level: "info"})
	}()

	reader := bufio.NewReader(resp.Body)
	ev := mustReadNextSSEEvent(t, reader)
	cancel()

	if ev.RunID != "r3" {
		t.Fatalf("expected runId=r3, got %q", ev.RunID)
	}
	if ev.Seq != 1 {
		t.Fatalf("expected seq=1, got %d", ev.Seq)
	}
}

func mustReadNextSSEEvent(t *testing.T, r *bufio.Reader) StreamEvent {
	t.Helper()

	type result struct {
		ev  StreamEvent
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ev, err := readNextSSEEvent(r)
		ch <- result{ev: ev, err: err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("read SSE error: %v", res.err)
		}
		return res.ev
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for SSE event")
		return StreamEvent{}
	}
}

func readNextSSEEvent(r *bufio.Reader) (StreamEvent, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return StreamEvent{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		raw := strings.TrimPrefix(line, "data: ")
		var ev StreamEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			return StreamEvent{}, err
		}

		// Consume the trailing blank line after the data line.
		_, _ = r.ReadString('\n')
		return ev, nil
	}
}
