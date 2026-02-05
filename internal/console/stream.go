package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const DefaultMaxEventsPerRun = 5000

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
	MaxEventsPerRun   int
	SubscriberBufSize int
}

type StreamHub struct {
	mu   sync.Mutex
	runs map[string]*streamRunState

	globalSubs map[chan StreamEvent]struct{}
	runSubs    map[string]map[chan StreamEvent]struct{}

	maxEventsPerRun    int
	subscriberBufSize  int
	emitReplayTruncate bool
}

type streamRunState struct {
	nextSeq uint64
	events  []StreamEvent

	replayTruncateEmitted bool
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

	return &StreamHub{
		runs:               make(map[string]*streamRunState),
		globalSubs:         make(map[chan StreamEvent]struct{}),
		runSubs:            make(map[string]map[chan StreamEvent]struct{}),
		maxEventsPerRun:    maxEvents,
		subscriberBufSize:  bufSize,
		emitReplayTruncate: true,
	}
}

func (h *StreamHub) Publish(event StreamEvent) StreamEvent {
	h.mu.Lock()
	event = h.publishLocked(event)
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

	for _, ch := range globalChans {
		select {
		case ch <- event:
		default:
		}
	}
	for _, ch := range runChans {
		select {
		case ch <- event:
		default:
		}
	}

	return event
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
