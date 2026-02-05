package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const DefaultMaxEventsPerRun = 5000
const DefaultMaxProcessTextBytes = 8 * 1024
const DefaultMaxArchiveBytes = 50 * 1024 * 1024
const DefaultArchiveRetentionCount = 50
const DefaultArchiveRetentionBytes = 1024 * 1024 * 1024

type StreamEvent struct {
	TS    string `json:"ts"`
	Seq   uint64 `json:"seq,omitempty"`
	RunID string `json:"runId,omitempty"`
	Type  string `json:"type"`
	Step  string `json:"step,omitempty"`
	Level string `json:"level,omitempty"`
	Data  any    `json:"data,omitempty"`
}

type StreamHubConfig struct {
	MaxEventsPerRun     int
	SubscriberBufSize   int
	MaxProcessTextBytes int
	ArchiveDir          string
	MaxArchiveBytes     int64
}

type StreamHub struct {
	mu   sync.Mutex
	runs map[string]*streamRunState

	globalSubs map[chan StreamEvent]struct{}
	runSubs    map[string]map[chan StreamEvent]struct{}

	maxEventsPerRun       int
	subscriberBufSize     int
	emitReplayTruncate    bool
	maxProcessTextBytes   int
	emitGovernanceWarning bool

	archiveDir      string
	maxArchiveBytes int64
	archives        map[string]*runArchiveState
}

type streamRunState struct {
	nextSeq uint64
	events  []StreamEvent

	replayTruncateEmitted    bool
	governanceWarningEmitted bool
}

type runArchiveState struct {
	tmpPath   string
	finalPath string
	f         *os.File

	bytesWritten int64
	stopped      bool
	errorEmitted bool
	finalized    bool

	cleanupAttempted    bool
	cleanupErrorEmitted bool
}

func NewStreamHub(cfg StreamHubConfig) *StreamHub {
	maxEvents := cfg.MaxEventsPerRun
	if maxEvents <= 0 {
		maxEvents = DefaultMaxEventsPerRun
	}
	bufSize := cfg.SubscriberBufSize
	if bufSize <= 0 {
		bufSize = 128
	}
	maxTextBytes := cfg.MaxProcessTextBytes
	if maxTextBytes <= 0 {
		maxTextBytes = DefaultMaxProcessTextBytes
	}
	archiveDir := cfg.ArchiveDir
	maxArchiveBytes := cfg.MaxArchiveBytes
	if archiveDir != "" && maxArchiveBytes <= 0 {
		maxArchiveBytes = DefaultMaxArchiveBytes
	}
	if archiveDir == "" {
		maxArchiveBytes = 0
	}

	return &StreamHub{
		runs:                  make(map[string]*streamRunState),
		globalSubs:            make(map[chan StreamEvent]struct{}),
		runSubs:               make(map[string]map[chan StreamEvent]struct{}),
		maxEventsPerRun:       maxEvents,
		subscriberBufSize:     bufSize,
		emitReplayTruncate:    true,
		maxProcessTextBytes:   maxTextBytes,
		emitGovernanceWarning: true,
		archiveDir:            archiveDir,
		maxArchiveBytes:       maxArchiveBytes,
		archives:              make(map[string]*runArchiveState),
	}
}

func (h *StreamHub) Publish(event StreamEvent) StreamEvent {
	h.mu.Lock()
	event, extra := h.governAndPublishLocked(event)
	runID := event.RunID

	globalChans := make([]chan StreamEvent, 0, len(h.globalSubs))
	for ch := range h.globalSubs {
		globalChans = append(globalChans, ch)
	}

	var runChans []chan StreamEvent
	if runID != "" {
		if subs, ok := h.runSubs[runID]; ok {
			runChans = make([]chan StreamEvent, 0, len(subs))
			for ch := range subs {
				runChans = append(runChans, ch)
			}
		}
	}
	h.mu.Unlock()

	events := make([]StreamEvent, 0, 1+len(extra))
	events = append(events, event)
	events = append(events, extra...)
	for _, ev := range events {
		for _, ch := range globalChans {
			select {
			case ch <- ev:
			default:
			}
		}
		for _, ch := range runChans {
			select {
			case ch <- ev:
			default:
			}
		}
	}

	return event
}

func (h *StreamHub) governAndPublishLocked(event StreamEvent) (published StreamEvent, extra []StreamEvent) {
	event, didTruncate := h.governProcessOutput(event)
	var archiveExtra *StreamEvent
	published, archiveExtra = h.publishLocked(event)
	if archiveExtra != nil {
		ev, _ := h.publishLocked(*archiveExtra)
		extra = append(extra, ev)
	}

	if !didTruncate || published.RunID == "" || !h.emitGovernanceWarning {
		return published, extra
	}

	state := h.runs[published.RunID]
	if state == nil {
		state = &streamRunState{}
		h.runs[published.RunID] = state
	}
	if state.governanceWarningEmitted {
		return published, nil
	}
	state.governanceWarningEmitted = true

	warning := StreamEvent{
		RunID: published.RunID,
		Type:  "progress",
		Step:  "governance",
		Level: "warn",
		Data: map[string]any{
			"phase":            "warning",
			"completeDetected": false,
			"note":             fmt.Sprintf("process output truncated to %d bytes per message", h.maxProcessTextBytes),
			"limitBytes":       h.maxProcessTextBytes,
		},
	}
	var warningArchiveExtra *StreamEvent
	warning, warningArchiveExtra = h.publishLocked(warning)
	extra = append(extra, warning)
	if warningArchiveExtra != nil {
		ev, _ := h.publishLocked(*warningArchiveExtra)
		extra = append(extra, ev)
	}
	return published, extra
}

func (h *StreamHub) governProcessOutput(event StreamEvent) (StreamEvent, bool) {
	if event.Type != "process_stdout" && event.Type != "process_stderr" {
		return event, false
	}
	data, ok := event.Data.(map[string]any)
	if !ok {
		return event, false
	}
	rawText, ok := data["text"].(string)
	if !ok {
		return event, false
	}
	truncatedText, didTruncate := truncateUTF8ToBytes(rawText, h.maxProcessTextBytes)
	if !didTruncate {
		return event, false
	}

	copied := make(map[string]any, len(data)+3)
	for k, v := range data {
		copied[k] = v
	}
	copied["text"] = truncatedText
	copied["truncated"] = true
	copied["originalBytes"] = len([]byte(rawText))
	copied["limitBytes"] = h.maxProcessTextBytes
	event.Data = copied
	return event, true
}

func truncateUTF8ToBytes(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", len(s) > 0
	}
	if len(s) <= maxBytes {
		return s, false
	}
	b := []byte(s)
	if maxBytes > len(b) {
		return s, false
	}
	cut := b[:maxBytes]
	for len(cut) > 0 && !utf8.Valid(cut) {
		cut = cut[:len(cut)-1]
	}
	return string(cut), true
}

func (h *StreamHub) publishLocked(event StreamEvent) (StreamEvent, *StreamEvent) {
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if event.RunID == "" {
		return event, nil
	}

	state, ok := h.runs[event.RunID]
	if !ok {
		state = &streamRunState{}
		h.runs[event.RunID] = state
	}
	state.nextSeq++
	event.Seq = state.nextSeq

	state.events = append(state.events, event)
	if len(state.events) > h.maxEventsPerRun {
		state.events = state.events[len(state.events)-h.maxEventsPerRun:]
	}

	archiveExtra := h.archiveEventLocked(event)
	return event, archiveExtra
}

func (h *StreamHub) SubscribeAll() (<-chan StreamEvent, func()) {
	ch := make(chan StreamEvent, h.subscriberBufSize)
	h.mu.Lock()
	h.globalSubs[ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.globalSubs, ch)
		h.mu.Unlock()
		close(ch)
	}
}

func (h *StreamHub) archiveEventLocked(event StreamEvent) *StreamEvent {
	if h.archiveDir == "" || h.maxArchiveBytes <= 0 {
		return nil
	}
	if event.RunID == "" {
		return nil
	}

	arch := h.archives[event.RunID]
	if arch == nil {
		safeName := sanitizeRunIDForFilename(event.RunID)
		if safeName == "" {
			safeName = "run"
		}
		finalPath := filepath.Join(h.archiveDir, safeName+".jsonl")
		tmpPath := finalPath + ".tmp"
		arch = &runArchiveState{tmpPath: tmpPath, finalPath: finalPath}
		h.archives[event.RunID] = arch
	}

	if arch.finalized {
		return nil
	}

	if arch.f == nil && !arch.stopped {
		if !arch.cleanupAttempted {
			arch.cleanupAttempted = true
			protected := make(map[string]struct{})
			for _, a := range h.archives {
				if a == nil || a.finalized || a.f == nil {
					continue
				}
				if a.tmpPath != "" {
					protected[a.tmpPath] = struct{}{}
				}
				if a.finalPath != "" {
					protected[a.finalPath] = struct{}{}
				}
			}

			maxFiles := DefaultArchiveRetentionCount
			if maxFiles > 0 {
				maxFiles-- // reserve a slot for this new archive.
			}
			maxBytes := int64(DefaultArchiveRetentionBytes)
			if h.maxArchiveBytes > 0 && maxBytes > h.maxArchiveBytes {
				maxBytes -= h.maxArchiveBytes // reserve max archive headroom.
			} else if h.maxArchiveBytes > 0 {
				maxBytes = 0
			}

			if err := cleanupArchiveDir(h.archiveDir, maxFiles, maxBytes, protected); err != nil && !arch.cleanupErrorEmitted {
				arch.cleanupErrorEmitted = true
				return &StreamEvent{
					RunID: event.RunID,
					Type:  "error",
					Step:  "archive",
					Level: "error",
					Data: map[string]any{
						"code":    "ARCHIVE_CLEANUP_FAILED",
						"message": "Failed to cleanup old run archives; continuing without blocking this run.",
						"dir":     h.archiveDir,
						"error":   err.Error(),
					},
				}
			}
		}

		if err := os.MkdirAll(h.archiveDir, 0o755); err != nil {
			arch.stopped = true
			if !arch.errorEmitted {
				arch.errorEmitted = true
				return &StreamEvent{
					RunID: event.RunID,
					Type:  "error",
					Step:  "archive",
					Level: "error",
					Data: map[string]any{
						"code":    "ARCHIVE_CREATE_FAILED",
						"message": "Failed to create archive directory; run logs will not be written to disk.",
						"dir":     h.archiveDir,
					},
				}
			}
			return nil
		}

		f, err := os.OpenFile(arch.tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			arch.stopped = true
			if !arch.errorEmitted {
				arch.errorEmitted = true
				return &StreamEvent{
					RunID: event.RunID,
					Type:  "error",
					Step:  "archive",
					Level: "error",
					Data: map[string]any{
						"code":    "ARCHIVE_OPEN_FAILED",
						"message": "Failed to open run archive; run logs will not be written to disk.",
						"path":    arch.tmpPath,
					},
				}
			}
			return nil
		}
		arch.f = f
	}

	if arch.stopped || arch.f == nil {
		if event.Type != "run_finished" {
			return nil
		}

		if arch.f != nil {
			if err := arch.f.Close(); err != nil && !arch.errorEmitted {
				arch.errorEmitted = true
				arch.finalized = true
				arch.f = nil
				return &StreamEvent{
					RunID: event.RunID,
					Type:  "error",
					Step:  "archive",
					Level: "error",
					Data: map[string]any{
						"code":    "ARCHIVE_CLOSE_FAILED",
						"message": "Failed to close run archive; archive may be incomplete.",
						"path":    arch.tmpPath,
					},
				}
			}
			arch.f = nil
			if err := os.Rename(arch.tmpPath, arch.finalPath); err != nil && !arch.errorEmitted {
				arch.errorEmitted = true
				arch.finalized = true
				return &StreamEvent{
					RunID: event.RunID,
					Type:  "error",
					Step:  "archive",
					Level: "error",
					Data: map[string]any{
						"code":    "ARCHIVE_RENAME_FAILED",
						"message": "Failed to finalize run archive; archive may be left as a .tmp file.",
						"tmpPath": arch.tmpPath,
						"path":    arch.finalPath,
					},
				}
			}
		}

		arch.finalized = true
		return nil
	}

	line, err := encodeJSONLLine(event)
	if err != nil {
		arch.stopped = true
		if !arch.errorEmitted {
			arch.errorEmitted = true
			return &StreamEvent{
				RunID: event.RunID,
				Type:  "error",
				Step:  "archive",
				Level: "error",
				Data: map[string]any{
					"code":    "ARCHIVE_ENCODE_FAILED",
					"message": "Failed to encode an event for archiving; further events will not be written to disk.",
				},
			}
		}
		return nil
	}

	if arch.bytesWritten+int64(len(line)) > h.maxArchiveBytes {
		arch.stopped = true
		if !arch.errorEmitted {
			arch.errorEmitted = true
			return &StreamEvent{
				RunID: event.RunID,
				Type:  "error",
				Step:  "archive",
				Level: "error",
				Data: map[string]any{
					"code":     "ARCHIVE_TOO_LARGE",
					"message":  "Run archive exceeded the configured max size; further events will not be written to disk.",
					"maxBytes": h.maxArchiveBytes,
					"path":     arch.finalPath,
				},
			}
		}
		return nil
	}

	if _, err := arch.f.Write(line); err != nil {
		arch.stopped = true
		if !arch.errorEmitted {
			arch.errorEmitted = true
			return &StreamEvent{
				RunID: event.RunID,
				Type:  "error",
				Step:  "archive",
				Level: "error",
				Data: map[string]any{
					"code":    "ARCHIVE_WRITE_FAILED",
					"message": "Failed to write run archive; further events will not be written to disk.",
					"path":    arch.tmpPath,
				},
			}
		}
		return nil
	}
	arch.bytesWritten += int64(len(line))

	if event.Type == "run_finished" {
		if err := arch.f.Close(); err != nil && !arch.errorEmitted {
			arch.errorEmitted = true
			arch.finalized = true
			arch.f = nil
			return &StreamEvent{
				RunID: event.RunID,
				Type:  "error",
				Step:  "archive",
				Level: "error",
				Data: map[string]any{
					"code":    "ARCHIVE_CLOSE_FAILED",
					"message": "Failed to close run archive; archive may be incomplete.",
					"path":    arch.tmpPath,
				},
			}
		}
		arch.f = nil
		if err := os.Rename(arch.tmpPath, arch.finalPath); err != nil && !arch.errorEmitted {
			arch.errorEmitted = true
			arch.finalized = true
			return &StreamEvent{
				RunID: event.RunID,
				Type:  "error",
				Step:  "archive",
				Level: "error",
				Data: map[string]any{
					"code":    "ARCHIVE_RENAME_FAILED",
					"message": "Failed to finalize run archive; archive may be left as a .tmp file.",
					"tmpPath": arch.tmpPath,
					"path":    arch.finalPath,
				},
			}
		}
		arch.finalized = true
	}

	return nil
}

type archiveFileEntry struct {
	path    string
	modTime time.Time
	size    int64
}

func cleanupArchiveDir(dir string, maxFiles int, maxTotalBytes int64, protected map[string]struct{}) error {
	if dir == "" {
		return nil
	}
	if maxFiles < 0 {
		maxFiles = 0
	}
	if maxTotalBytes < 0 {
		maxTotalBytes = 0
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	files := make([]archiveFileEntry, 0, len(entries))
	var total int64

	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		if protected != nil {
			if _, ok := protected[path]; ok {
				continue
			}
		}

		info, err := os.Lstat(path)
		if err != nil {
			// Best-effort: ignore races (e.g., file removed concurrently).
			continue
		}
		if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			size := info.Size()
			files = append(files, archiveFileEntry{path: path, modTime: info.ModTime(), size: size})
			total += size
		}
	}

	if len(files) == 0 {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.Before(files[j].modTime)
	})

	var firstErr error
	for (maxFiles > 0 && len(files) > maxFiles) || (maxTotalBytes > 0 && total > maxTotalBytes) {
		oldest := files[0]
		files = files[1:]
		if err := os.Remove(oldest.path); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		total -= oldest.size
	}

	if firstErr != nil {
		return firstErr
	}
	// If we couldn't reach limits (e.g., due to remove failures), still surface an error.
	if (maxFiles > 0 && len(files) > maxFiles) || (maxTotalBytes > 0 && total > maxTotalBytes) {
		return fmt.Errorf("archive cleanup incomplete: remainingFiles=%d remainingBytes=%d", len(files), total)
	}
	return nil
}

func sanitizeRunIDForFilename(runID string) string {
	if runID == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(runID))
	for _, r := range runID {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(outMaybeTrimDots(b.String()), " _")
	out = strings.Trim(out, ".")
	if out == "" {
		return ""
	}
	if out == "." || out == ".." {
		return ""
	}
	return out
}

func outMaybeTrimDots(s string) string {
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	return s
}

func encodeJSONLLine(ev StreamEvent) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ev); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return []byte("\n"), nil
	}
	// Encoder.Encode already appends a newline, which is exactly JSONL.
	return buf.Bytes(), nil
}

func (h *StreamHub) ReplayAndSubscribe(runID string, sinceSeq uint64) (replay []StreamEvent, ch <-chan StreamEvent, unsubscribe func(), truncated bool) {
	if runID == "" {
		events, unsub := h.SubscribeAll()
		return nil, events, unsub, false
	}

	subCh := make(chan StreamEvent, h.subscriberBufSize)

	h.mu.Lock()
	state, ok := h.runs[runID]
	if !ok {
		state = &streamRunState{}
		h.runs[runID] = state
	}

	if len(state.events) > 0 {
		minSeq := state.events[0].Seq
		if minSeq > sinceSeq+1 {
			truncated = true
		}
	}

	for _, ev := range state.events {
		if ev.Seq > sinceSeq {
			replay = append(replay, ev)
		}
	}

	if h.runSubs[runID] == nil {
		h.runSubs[runID] = make(map[chan StreamEvent]struct{})
	}
	h.runSubs[runID][subCh] = struct{}{}
	h.mu.Unlock()

	return replay, subCh, func() {
		h.mu.Lock()
		if subs, ok := h.runSubs[runID]; ok {
			delete(subs, subCh)
			if len(subs) == 0 {
				delete(h.runSubs, runID)
			}
		}
		h.mu.Unlock()
		close(subCh)
	}, truncated
}

func (h *StreamHub) MaybeEmitReplayTruncated(runID string) {
	if runID == "" || !h.emitReplayTruncate {
		return
	}

	h.mu.Lock()
	state, ok := h.runs[runID]
	if !ok {
		state = &streamRunState{}
		h.runs[runID] = state
	}
	if state.replayTruncateEmitted {
		h.mu.Unlock()
		return
	}
	state.replayTruncateEmitted = true
	h.mu.Unlock()

	h.Publish(StreamEvent{
		RunID: runID,
		Type:  "progress",
		Step:  "stream",
		Level: "warn",
		Data: map[string]any{
			"phase":            "error",
			"completeDetected": false,
			"note":             "replay truncated; some events missing",
		},
	})
}

func StreamHandler(hub *StreamHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := r.URL.Query().Get("runId")
		sinceSeq := uint64(0)
		if runID != "" {
			if raw := r.URL.Query().Get("sinceSeq"); raw != "" {
				n, err := strconv.ParseUint(raw, 10, 64)
				if err != nil {
					WriteAPIError(w, http.StatusBadRequest, APIError{
						Code:    "INVALID_QUERY",
						Message: "sinceSeq must be an unsigned integer.",
						Hint:    "Use /api/stream?runId=<id>&sinceSeq=<n>.",
					})
					return
				}
				sinceSeq = n
			}
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteAPIError(w, http.StatusInternalServerError, APIError{
				Code:    "STREAM_UNSUPPORTED",
				Message: "Streaming is not supported by this server.",
			})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(": ok\n\n"))
		flusher.Flush()

		replay, ch, unsubscribe, truncated := hub.ReplayAndSubscribe(runID, sinceSeq)
		defer unsubscribe()

		for _, ev := range replay {
			if err := writeSSEData(w, ev); err != nil {
				return
			}
			flusher.Flush()
		}

		if truncated {
			hub.MaybeEmitReplayTruncated(runID)
		}

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if err := writeSSEData(w, ev); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func writeSSEData(w http.ResponseWriter, ev StreamEvent) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ev); err != nil {
		return err
	}
	payload := bytes.TrimRight(buf.Bytes(), "\n")
	if bytes.Contains(payload, []byte("\n")) {
		return fmt.Errorf("SSE payload must be single-line JSON")
	}

	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err := w.Write([]byte("\n\n"))
	return err
}
