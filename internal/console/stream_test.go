package console

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStreamHub_TruncatesProcessOutputAndEmitsGovernanceWarningOnce(t *testing.T) {
	hub := NewStreamHub(StreamHubConfig{MaxEventsPerRun: 100, SubscriberBufSize: 16, MaxProcessTextBytes: 5})
	ch, unsub := hub.SubscribeAll()
	t.Cleanup(unsub)

	hub.Publish(StreamEvent{
		RunID: "r1",
		Type:  "process_stdout",
		Step:  "fire",
		Level: "info",
		Data:  map[string]any{"text": "hello world"},
	})

	ev1 := mustReadStreamEvent(t, ch)
	if ev1.Type != "process_stdout" {
		t.Fatalf("expected process_stdout, got %q", ev1.Type)
	}
	data1, ok := ev1.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", ev1.Data)
	}
	text1, _ := data1["text"].(string)
	if len(text1) > 5 {
		t.Fatalf("expected text <= 5 bytes, got %d", len(text1))
	}
	if data1["truncated"] != true {
		t.Fatalf("expected truncated=true, got %v", data1["truncated"])
	}

	ev2 := mustReadStreamEvent(t, ch)
	if ev2.Type != "progress" || ev2.Step != "governance" {
		t.Fatalf("expected governance progress warning, got type=%q step=%q", ev2.Type, ev2.Step)
	}

	hub.Publish(StreamEvent{
		RunID: "r1",
		Type:  "process_stderr",
		Step:  "fire",
		Level: "info",
		Data:  map[string]any{"text": "123456789"},
	})

	ev3 := mustReadStreamEvent(t, ch)
	if ev3.Type != "process_stderr" {
		t.Fatalf("expected process_stderr, got %q", ev3.Type)
	}

	select {
	case ev := <-ch:
		t.Fatalf("unexpected extra event after second truncated message: type=%q", ev.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

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

func TestStreamHub_ArchivesRunToJSONLAndRenamesOnFinish(t *testing.T) {
	archiveDir := filepath.Join(t.TempDir(), "runs")
	hub := NewStreamHub(StreamHubConfig{
		MaxEventsPerRun:   100,
		SubscriberBufSize: 16,
		ArchiveDir:        archiveDir,
	})

	hub.Publish(StreamEvent{RunID: "r-arch", Type: "run_started", Step: "fire", Level: "info"})
	tmpPath := filepath.Join(archiveDir, "r-arch.jsonl.tmp")
	finalPath := filepath.Join(archiveDir, "r-arch.jsonl")

	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("expected tmp archive to exist, got stat error: %v", err)
	}
	if _, err := os.Stat(finalPath); err == nil {
		t.Fatalf("expected final archive to not exist yet")
	}

	hub.Publish(StreamEvent{RunID: "r-arch", Type: "process_stdout", Step: "fire", Level: "info", Data: map[string]any{"text": "hello"}})
	hub.Publish(StreamEvent{RunID: "r-arch", Type: "run_finished", Step: "fire", Level: "info"})

	if _, err := os.Stat(tmpPath); err == nil {
		t.Fatalf("expected tmp archive to be renamed on finish")
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("expected final archive to exist, got stat error: %v", err)
	}

	raw, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 JSONL lines, got %d", len(lines))
	}

	for i, line := range lines {
		var ev StreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal line %d: %v", i, err)
		}
		if ev.RunID != "r-arch" {
			t.Fatalf("expected runId=r-arch, got %q", ev.RunID)
		}
	}
}

func TestStreamHub_ArchiveMaxSizeStopsWritingAndEmitsErrorEvent(t *testing.T) {
	archiveDir := filepath.Join(t.TempDir(), "runs")
	hub := NewStreamHub(StreamHubConfig{
		MaxEventsPerRun:   500,
		SubscriberBufSize: 16,
		ArchiveDir:        archiveDir,
		MaxArchiveBytes:   300,
	})

	hub.Publish(StreamEvent{RunID: "r-limit", Type: "run_started", Step: "fire", Level: "info"})
	for i := 0; i < 20; i++ {
		hub.Publish(StreamEvent{
			RunID: "r-limit",
			Type:  "process_stdout",
			Step:  "fire",
			Level: "info",
			Data:  map[string]any{"text": strings.Repeat("x", 200)},
		})
	}
	hub.Publish(StreamEvent{RunID: "r-limit", Type: "run_finished", Step: "fire", Level: "info"})

	replay, _, unsub, _ := hub.ReplayAndSubscribe("r-limit", 0)
	unsub()

	var found bool
	for _, ev := range replay {
		if ev.Type != "error" || ev.Step != "archive" {
			continue
		}
		data, ok := ev.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected error event data object, got %T", ev.Data)
		}
		if data["code"] == "ARCHIVE_TOO_LARGE" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an ARCHIVE_TOO_LARGE error event in replay")
	}

	finalPath := filepath.Join(archiveDir, "r-limit.jsonl")
	info, err := os.Stat(finalPath)
	if err != nil {
		t.Fatalf("expected final archive to exist, got stat error: %v", err)
	}
	if info.Size() > 300 {
		t.Fatalf("expected archive size <= 300 bytes, got %d", info.Size())
	}
}

func TestStreamHub_ArchiveCleanupReservesSlotForNewArchive(t *testing.T) {
	archiveDir := filepath.Join(t.TempDir(), "runs")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	// Create 55 archives with increasing mtimes (oldest first).
	base := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 55; i++ {
		name := filepath.Join(archiveDir, "r"+strconv.Itoa(i)+".jsonl")
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		ts := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(name, ts, ts); err != nil {
			t.Fatalf("Chtimes %s: %v", name, err)
		}
	}

	hub := NewStreamHub(StreamHubConfig{ArchiveDir: archiveDir})
	hub.Publish(StreamEvent{RunID: "new", Type: "run_started", Step: "fire", Level: "info"})

	ents, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	var files int
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		files++
	}
	if files != DefaultArchiveRetentionCount {
		t.Fatalf("expected %d files after cleanup+new archive, got %d", DefaultArchiveRetentionCount, files)
	}

	if _, err := os.Stat(filepath.Join(archiveDir, "r0.jsonl")); err == nil {
		t.Fatalf("expected oldest archive to be removed")
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "r5.jsonl")); err == nil {
		t.Fatalf("expected oldest archives to be removed")
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "r6.jsonl")); err != nil {
		t.Fatalf("expected newer archive to remain, stat error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "r54.jsonl")); err != nil {
		t.Fatalf("expected newest archive to remain, stat error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(archiveDir, "new.jsonl.tmp")); err != nil {
		t.Fatalf("expected new tmp archive to exist, stat error: %v", err)
	}
}

func TestCleanupArchiveDir_DeletesOldestUntilSizeWithinLimit(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-1 * time.Hour)

	paths := []string{
		filepath.Join(dir, "a.jsonl"),
		filepath.Join(dir, "b.jsonl"),
		filepath.Join(dir, "c.jsonl"),
	}
	for i, p := range paths {
		if err := os.WriteFile(p, []byte(strings.Repeat("x", 10)), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
		ts := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatalf("Chtimes %s: %v", p, err)
		}
	}

	if err := cleanupArchiveDir(dir, 10, 15, nil); err != nil {
		t.Fatalf("cleanupArchiveDir error: %v", err)
	}
	if _, err := os.Stat(paths[0]); err == nil {
		t.Fatalf("expected oldest file to be removed")
	}
	if _, err := os.Stat(paths[1]); err == nil {
		t.Fatalf("expected middle file to be removed")
	}
	if _, err := os.Stat(paths[2]); err != nil {
		t.Fatalf("expected newest file to remain, stat error: %v", err)
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

func mustReadStreamEvent(t *testing.T, ch <-chan StreamEvent) StreamEvent {
	t.Helper()

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatalf("stream channel closed")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for stream event")
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
