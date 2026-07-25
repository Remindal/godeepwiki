package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"deepwiki/internal/eventbus"
	"deepwiki/internal/model"
)

// EventHandler GET /api/v1/events（SSE）与 GET /api/v1/ws（WebSocket）。
type EventHandler struct {
	bus      eventbus.EventBus
	replayer eventbus.Replayer // Redis Streams XRANGE 回放（与 bus 同实例）
	upgrader websocket.Upgrader
	logger   *zap.Logger
}

func NewEventHandler(bus eventbus.EventBus, replayer eventbus.Replayer, allowedOrigins []string, logger *zap.Logger) *EventHandler {
	return &EventHandler{
		bus:      bus,
		replayer: replayer,
		logger:   logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: wsOriginChecker(allowedOrigins),
		},
	}
}

// wsOriginChecker 按 server.cors_allowed_origins 白名单校验 WS Origin（硬约束 #12）。
// 无 Origin 头（非浏览器客户端）放行；白名单为空（dev 未配置）放行。
func wsOriginChecker(allowed []string) func(r *http.Request) bool {
	whitelist := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		whitelist[strings.ToLower(strings.TrimSpace(o))] = struct{}{}
	}
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" || len(whitelist) == 0 {
			return true
		}
		_, ok := whitelist[strings.ToLower(origin)]
		return ok
	}
}

func parseEventFilter(c *gin.Context) model.EventFilter {
	var filter model.EventFilter
	if types := c.Query("types"); types != "" {
		for _, t := range strings.Split(types, ",") {
			if t = strings.TrimSpace(t); t != "" {
				filter.Types = append(filter.Types, t)
			}
		}
	}
	filter.RepoID = c.Query("repo_id")
	filter.TaskID = c.Query("task_id")
	return filter
}

func writeSSE(w io.Writer, f http.Flusher, id uint64, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, event, data); err != nil {
		return err
	}
	f.Flush()
	return nil
}

func (h *EventHandler) Events(c *gin.Context) {
	filter := parseEventFilter(c)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondError(c, model.CodeInternalError, "streaming not supported", nil)
		return
	}
	ctx := c.Request.Context()

	// Last-Event-ID 回放（Redis Streams XRANGE）。
	if lastID := c.GetHeader("Last-Event-ID"); lastID != "" {
		if seq, err := strconv.ParseUint(lastID, 10, 64); err == nil && seq > 0 {
			events, ok := h.replayer.ReplaySince(ctx, seq, filter)
			if !ok {
				_ = writeSSE(c.Writer, flusher, seq, model.EventTypeGap, gin.H{"message": "event history truncated"})
			} else {
				for _, ev := range events {
					if err := writeSSE(c.Writer, flusher, ev.Seq, ev.Type, ev); err != nil {
						return
					}
				}
			}
		}
	}

	ch, cancel := h.bus.Subscribe(filter)
	defer cancel()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := c.Writer.WriteString(": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(c.Writer, flusher, ev.Seq, ev.Type, ev); err != nil {
				return
			}
		}
	}
}

func (h *EventHandler) WebSocket(c *gin.Context) {
	filter := parseEventFilter(c)
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn("websocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// resume_from 回放。
	if resume := c.Query("resume_from"); resume != "" {
		if seq, err := strconv.ParseUint(resume, 10, 64); err == nil && seq > 0 {
			events, ok := h.replayer.ReplaySince(ctx, seq, filter)
			if !ok {
				_ = writeWSJSON(conn, wsFrame{Seq: seq, Type: model.EventTypeGap, Data: json.RawMessage(`{"message":"event history truncated"}`)})
			} else {
				for _, ev := range events {
					if err := writeWSEvent(conn, ev); err != nil {
						return
					}
				}
			}
		}
	}

	ch, unsub := h.bus.Subscribe(filter)
	defer unsub()

	// 读侧仅用于感知客户端断开。
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-readDone:
			return
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeWSEvent(conn, ev); err != nil {
				return
			}
		}
	}
}

type wsFrame struct {
	Seq  uint64          `json:"seq"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

var wsWriteMu sync.Mutex

func writeWSJSON(conn *websocket.Conn, frame wsFrame) error {
	wsWriteMu.Lock()
	defer wsWriteMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(frame)
}

func writeWSEvent(conn *websocket.Conn, ev model.Event) error {
	data := ev.Payload
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	return writeWSJSON(conn, wsFrame{Seq: ev.Seq, Type: ev.Type, Data: data})
}
