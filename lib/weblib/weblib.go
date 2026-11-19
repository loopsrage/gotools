package weblib

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// WriteJSON encodes v as JSON with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError encodes a FastAPI-compatible `{"detail": msg}` JSON error response.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]any{"detail": msg, "error": msg})
}

// ReadJSON decodes JSON from r.Body into dst with a 16 MiB safety bound.
func ReadJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

// CORSMiddleware permits cross-origin requests with credentials for local/UI deployments.
func CORSMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				for _, o := range allowedOrigins {
					if o == "*" || o == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						break
					}
				}
			}
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With, X-Agent-Id")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WSConn wraps coder/websocket.Conn with ergonomic JSON helpers.
type WSConn struct {
	Conn *websocket.Conn
}

// AcceptWebSocket upgrades an HTTP request to an RFC 6455 WebSocket connection.
func AcceptWebSocket(w http.ResponseWriter, r *http.Request) (*WSConn, error) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(16 * 1024 * 1024)
	return &WSConn{Conn: c}, nil
}

// ReadJSON reads a JSON message from the WebSocket.
func (ws *WSConn) ReadJSON(ctx context.Context, dst any) error {
	return wsjson.Read(ctx, ws.Conn, dst)
}

// WriteJSON writes a JSON message to the WebSocket.
func (ws *WSConn) WriteJSON(ctx context.Context, v any) error {
	return wsjson.Write(ctx, ws.Conn, v)
}

// Close closes the WebSocket normally.
func (ws *WSConn) Close() error {
	return ws.Conn.Close(websocket.StatusNormalClosure, "closed")
}

// SPAHandler serves a built Vite/React SPA directory (distDir) under mountPrefix (e.g. "/ui"),
// serving static files directly and falling back to index.html for client-side routes.
func SPAHandler(distDir, mountPrefix string) http.Handler {
	cleanPrefix := strings.TrimRight(mountPrefix, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := r.URL.Path
		if cleanPrefix != "" && strings.HasPrefix(rel, cleanPrefix) {
			rel = strings.TrimPrefix(rel, cleanPrefix)
		}
		rel = strings.TrimLeft(rel, "/")
		if rel == "" {
			rel = "index.html"
		}
		// Prevent directory traversal
		cleanRel := filepath.Clean(rel)
		if strings.HasPrefix(cleanRel, "..") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		full := filepath.Join(distDir, cleanRel)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			http.ServeFile(w, r, full)
			return
		}
		// SPA fallback to index.html
		indexFile := filepath.Join(distDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			http.ServeFile(w, r, indexFile)
			return
		}
		http.NotFound(w, r)
	})
}
