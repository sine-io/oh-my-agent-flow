package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

const DefaultMaxEventsPerRun = 5000
const DefaultMaxProcessTextBytes = 8 * 1024

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
}

type streamRunState struct {
	nextSeq uint64
	events  []StreamEvent

	replayTruncateEmitted    bool
	governanceWarningEmitted bool
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

	return &StreamHub{
		runs:                  make(map[string]*streamRunState),
		globalSubs:            make(map[chan StreamEvent]struct{}),
		runSubs:               make(map[string]map[chan StreamEvent]struct{}),
		maxEventsPerRun:       maxEvents,
		subscriberBufSize:     bufSize,
		emitReplayTruncate:    true,
		maxProcessTextBytes:   maxTextBytes,
		emitGovernanceWarning: true,
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
	published = h.publishLocked(event)

	if !didTruncate || published.RunID == "" || !h.emitGovernanceWarning {
		return published, nil
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
	warning = h.publishLocked(warning)
	return published, []StreamEvent{warning}
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

func (h *StreamHub) publishLocked(event StreamEvent) StreamEvent {
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if event.RunID == "" {
		return event
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

	return event
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
