package hub

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/shared/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type SnapshotFunc func() model.Snapshot

type Hub struct {
	mu       sync.Mutex
	clients  map[*websocket.Conn]struct{}
	snapshot SnapshotFunc
}

func New(snapshot SnapshotFunc) *Hub {
	return &Hub{
		clients:  map[*websocket.Conn]struct{}{},
		snapshot: snapshot,
	}
}

func (h *Hub) HandleLive(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)

	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()

	ctx := r.Context()
	if h.snapshot != nil {
		_ = wsjson.Write(ctx, conn, snapshotMessage(h.snapshot()))
	}

	go h.drain(ctx, conn)
	<-ctx.Done()
	h.remove(conn)
}

func (h *Hub) Broadcast(ctx context.Context, msg any) {
	h.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, conn := range clients {
		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := wsjson.Write(writeCtx, conn, msg)
		cancel()
		if err != nil {
			h.remove(conn)
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.clients = map[*websocket.Conn]struct{}{}
	h.mu.Unlock()
	for _, conn := range clients {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

func (h *Hub) drain(ctx context.Context, conn *websocket.Conn) {
	defer h.remove(conn)
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func (h *Hub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	if _, ok := h.clients[conn]; ok {
		delete(h.clients, conn)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	h.mu.Unlock()
}

func snapshotMessage(s model.Snapshot) protocol.Snapshot {
	projects := make([]any, 0, len(s.Projects))
	for _, p := range s.Projects {
		projects = append(projects, p)
	}
	nodes := make([]any, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		nodes = append(nodes, n)
	}
	return protocol.Snapshot{
		Type:     protocol.TypeSnapshot,
		Version:  protocol.Version,
		ServerTs: time.Now().Unix(),
		Projects: projects,
		Nodes:    nodes,
	}
}
