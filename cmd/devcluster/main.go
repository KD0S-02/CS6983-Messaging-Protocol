package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/gateway"
	ssdpkg "github.com/KD0S-02/cs6983-messaging-protocol/internal/ssd"
	"github.com/KD0S-02/cs6983-messaging-protocol/internal/transport"
)

const replicaCount = 3

type config struct {
	HTTPAddr    string
	DataDir     string
	Gateways    int
	SSDAddrs    string
	PeerAddrs   string
	LatencyMs   int
	JitterMs    int
	FailRate    float64
	MaxBodySize int64
}

type app struct {
	gateways map[uint8]*gateway.Gateway
	cfg      config
}

type gatewayInfo struct {
	ID       uint8  `json:"id"`
	PeerAddr string `json:"peer_addr"`
}

type clusterInfo struct {
	HTTPAddr string        `json:"http_addr"`
	SSDs     []string      `json:"ssds"`
	Gateways []gatewayInfo `json:"gateways"`
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func parseFlags() config {
	cfg := config{}

	flag.StringVar(&cfg.HTTPAddr, "http", "127.0.0.1:8080", "HTTP API address")
	flag.StringVar(&cfg.DataDir, "data", "./data/devcluster", "directory used by local SSD block files")
	flag.IntVar(&cfg.Gateways, "gateways", 2, "number of gateways to start")
	flag.StringVar(&cfg.SSDAddrs, "ssd-addrs", "127.0.0.1:9100,127.0.0.1:9101,127.0.0.1:9102", "comma-separated SSD server addresses; exactly 3 required")
	flag.StringVar(&cfg.PeerAddrs, "peer-addrs", "", "comma-separated gateway peer listener addresses; defaults to 127.0.0.1:9201..N")
	flag.IntVar(&cfg.LatencyMs, "latency-ms", 0, "fixed SSD latency in milliseconds")
	flag.IntVar(&cfg.JitterMs, "jitter-ms", 0, "additional random SSD latency in milliseconds")
	flag.Float64Var(&cfg.FailRate, "fail-rate", 0, "SSD injected failure probability from 0.0 to 1.0")
	flag.Int64Var(&cfg.MaxBodySize, "max-body-bytes", 10<<20, "maximum PUT body size")

	flag.Parse()
	return cfg
}

func run(cfg config) error {
	if cfg.Gateways < 1 || cfg.Gateways > 255 {
		return fmt.Errorf("gateways must be between 1 and 255")
	}
	if cfg.FailRate < 0 || cfg.FailRate > 1 {
		return fmt.Errorf("fail-rate must be between 0.0 and 1.0")
	}

	ssdAddrs, err := parseCSV(cfg.SSDAddrs, replicaCount)
	if err != nil {
		return fmt.Errorf("ssd-addrs: %w", err)
	}

	peerAddrs, err := peerAddresses(cfg.PeerAddrs, cfg.Gateways)
	if err != nil {
		return fmt.Errorf("peer-addrs: %w", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	ssdClients, err := startSSDs(cfg, ssdAddrs)
	if err != nil {
		return err
	}

	gateways := make(map[uint8]*gateway.Gateway, cfg.Gateways)

	for i := 0; i < cfg.Gateways; i++ {
		id := uint8(i + 1)

		peers := make([]*transport.PeerClient, 0, cfg.Gateways-1)
		for j, addr := range peerAddrs {
			peerID := uint8(j + 1)
			if peerID == id {
				continue
			}
			peers = append(peers, transport.NewPeerClient(addr))
		}

		gateways[id] = gateway.New(id, ssdClients, peers)
	}

	for i, addr := range peerAddrs {
		gw := gateways[uint8(i+1)]
		srv := gateway.NewPeerServer(gw, addr)

		go func(id int, srv *gateway.PeerServer) {
			if err := srv.ListenAndServe(); err != nil {
				log.Printf("gateway-%d peer server stopped: %v", id, err)
			}
		}(i+1, srv)
	}

	for _, addr := range peerAddrs {
		if err := waitForTCP(addr, 2*time.Second); err != nil {
			return err
		}
	}

	api := &app{
		gateways: gateways,
		cfg:      cfg,
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.routes(clusterInfoFromConfig(cfg.HTTPAddr, ssdAddrs, peerAddrs)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("HTTP API listening on http://%s", cfg.HTTPAddr)
	log.Printf("try: curl -X PUT --data 'hello' http://%s/gateways/1/kv/demo", cfg.HTTPAddr)
	log.Printf("then: curl http://%s/gateways/2/kv/demo", cfg.HTTPAddr)

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Printf("received %s; shutting down HTTP API", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return httpServer.Shutdown(ctx)

	case err := <-errCh:
		return err
	}
}

func startSSDs(cfg config, addrs []string) ([replicaCount]*transport.SSDClient, error) {
	var clients [replicaCount]*transport.SSDClient

	for i, addr := range addrs {
		dir := filepath.Join(cfg.DataDir, fmt.Sprintf("ssd-%d", i))

		disk, err := ssdpkg.New(uint8(i), dir, ssdpkg.Config{
			LatencyMs: cfg.LatencyMs,
			JitterMs:  cfg.JitterMs,
			FailRate:  cfg.FailRate,
		})
		if err != nil {
			return clients, fmt.Errorf("create ssd-%d: %w", i, err)
		}

		srv := ssdpkg.NewServer(disk, addr)

		go func(id int, srv *ssdpkg.Server) {
			if err := srv.ListenAndServe(); err != nil {
				log.Printf("ssd-%d server stopped: %v", id, err)
			}
		}(i, srv)

		clients[i] = transport.NewSSDClient(addr)
	}

	for _, addr := range addrs {
		if err := waitForTCP(addr, 2*time.Second); err != nil {
			return clients, err
		}
	}

	return clients, nil
}

func (a *app) routes(info clusterInfo) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	mux.HandleFunc("/gateways", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateways" || r.Method != http.MethodGet {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		writeJSON(w, http.StatusOK, info)
	})

	mux.HandleFunc("/kv/", func(w http.ResponseWriter, r *http.Request) {
		key, err := decodeKey(strings.TrimPrefix(r.URL.Path, "/kv/"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		a.handleKV(w, r, 1, key)
	})

	mux.HandleFunc("/gateways/", func(w http.ResponseWriter, r *http.Request) {
		id, key, err := parseGatewayKVPath(r.URL.Path)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		a.handleKV(w, r, id, key)
	})

	return mux
}

func (a *app) handleKV(w http.ResponseWriter, r *http.Request, gatewayID uint8, key string) {
	gw, ok := a.gateways[gatewayID]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("gateway %d not found", gatewayID))
		return
	}

	switch r.Method {
	case http.MethodPut, http.MethodPost:
		body := http.MaxBytesReader(w, r.Body, a.cfg.MaxBodySize)
		defer body.Close()

		value, err := io.ReadAll(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("read request body: %v", err))
			return
		}

		if err := gw.HandlePUT(key, value); err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"gateway": gatewayID,
			"key":     key,
			"bytes":   len(value),
		})

	case http.MethodGet:
		value, err := gw.HandleGET(key)
		if err != nil {
			if errors.Is(err, gateway.ErrNotFound) {
				writeError(w, http.StatusNotFound, "key not found")
				return
			}

			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(value)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func parseGatewayKVPath(path string) (uint8, string, error) {
	rest := strings.TrimPrefix(path, "/gateways/")
	parts := strings.SplitN(rest, "/", 3)

	if len(parts) != 3 || parts[1] != "kv" {
		return 0, "", fmt.Errorf("expected /gateways/{id}/kv/{key}")
	}

	id64, err := strconv.ParseUint(parts[0], 10, 8)
	if err != nil || id64 == 0 {
		return 0, "", fmt.Errorf("invalid gateway id %q", parts[0])
	}

	key, err := decodeKey(parts[2])
	if err != nil {
		return 0, "", err
	}

	return uint8(id64), key, nil
}

func decodeKey(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("key is required")
	}

	key, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid key: %w", err)
	}

	if key == "" {
		return "", fmt.Errorf("key is required")
	}

	return key, nil
}

func parseCSV(value string, want int) ([]string, error) {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}

	if len(out) != want {
		return nil, fmt.Errorf("got %d addresses, want %d", len(out), want)
	}

	for _, addr := range out {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return nil, fmt.Errorf("invalid address %q: %w", addr, err)
		}
	}

	return out, nil
}

func peerAddresses(value string, gateways int) ([]string, error) {
	if strings.TrimSpace(value) != "" {
		return parseCSV(value, gateways)
	}

	addrs := make([]string, gateways)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("127.0.0.1:%d", 9201+i)
	}

	return addrs, nil
}

func clusterInfoFromConfig(httpAddr string, ssdAddrs, peerAddrs []string) clusterInfo {
	gateways := make([]gatewayInfo, len(peerAddrs))

	for i, addr := range peerAddrs {
		gateways[i] = gatewayInfo{
			ID:       uint8(i + 1),
			PeerAddr: addr,
		}
	}

	return clusterInfo{
		HTTPAddr: httpAddr,
		SSDs:     ssdAddrs,
		Gateways: gateways,
	}
}

func waitForTCP(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		time.Sleep(25 * time.Millisecond)
	}

	return fmt.Errorf("server on %s did not start within %s", addr, timeout)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error":  message,
		"status": status,
	})
}
