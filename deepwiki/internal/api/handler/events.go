package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"deepwiki/internal/eventbus"
)

// EventHandler GET /api/v1/events（SSE）与 GET /api/v1/ws（WebSocket）。
type EventHandler struct {
	bus      eventbus.EventBus
	replayer eventbus.Replayer // Redis Streams XRANGE 回放（与 bus 同实例）
	upgrader websocket.Upgrader
	logger   *zap.Logger
}

func NewEventHandler(bus eventbus.EventBus, replayer eventbus.Replayer, logger *zap.Logger) *EventHandler {
	return &EventHandler{
		bus:      bus,
		replayer: replayer,
		logger:   logger,
		upgrader: websocket.Upgrader{
			// TODO（下一轮）：CheckOrigin 必须按 server.cors_allowed_origins 白名单校验（硬约束 #12）。
		},
	}
}

func (h *EventHandler) Events(c *gin.Context) {
	// TODO: GET /api/v1/events?types=task.state_changed,wiki.completed&repo_id=repo_...（§6.4）：
	// ① 仅订阅 EventBus（model.EventFilter{Types, RepoID}），禁止直接订阅 Task（建议⑪）；
	// ② Last-Event-ID 头 → h.replayer.ReplaySince 补发（Redis Streams：XRANGE events:task:<task_id> <last> +，
	//    每任务流 XTRIM MAXLEN ~ 1000）；过旧（流已截断）推 event: gap 提示回退 GET /api/v1/tasks；
	// ③ 帧格式：id: <seq> + event: <type> + data: <json>；每 15s 一行 ": heartbeat"；
	// ④ payload 为结构化字段直推（事件名与 payload 冻结，总纲 §2.5），禁止拼字符串（建议②）；
	// ⑤ 客户端断开即取消订阅并退出。
	respondNotImplemented(c)
}

func (h *EventHandler) WebSocket(c *gin.Context) {
	// TODO: GET /api/v1/ws（§6.7）：
	// ① upgrader.Upgrade 101；Query 过滤语义同 /events（types/repo_id）；
	// ② 推送 JSON 帧 {"seq":12,"type":"task.state_changed","data":{...}}（seq 单调递增 + resume_from 回放，
	//    回放源同为 Redis Streams）；
	// ③ 服务端每 15s 发 WS ping 帧；写超时与读超时必须设置，goroutine 带 recover（硬约束 #4）；
	// ④ 无 WS 内断线补发时，重连后客户端回退 GET /api/v1/tasks（§6.4）。
	respondNotImplemented(c)
}
