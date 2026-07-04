package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	nethttp "net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type AdminVNCProxyHandler struct {
	logger *slog.Logger
	store  AdminHostStore
}

func NewAdminVNCProxyHandler(logger *slog.Logger, store AdminHostStore) *AdminVNCProxyHandler {
	return &AdminVNCProxyHandler{logger: logger, store: store}
}

func (h *AdminVNCProxyHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	hostID := r.PathValue("hostID")
	if hostID == "" {
		nethttp.Error(w, "missing host ID", nethttp.StatusBadRequest)
		return
	}

	host, err := h.store.GetHost(r.Context(), hostID)
	if err != nil {
		nethttp.Error(w, "host not found", nethttp.StatusNotFound)
		return
	}
	if host.Status != "running" {
		nethttp.Error(w, "host is not running", nethttp.StatusConflict)
		return
	}

	containerName := fmt.Sprintf("claudedock-%s", hostID)
	containerIP, err := getContainerIP(r.Context(), containerName)
	if err != nil {
		h.logger.Error("get container IP failed", "container", containerName, "error", err)
		nethttp.Error(w, "cannot reach container", nethttp.StatusServiceUnavailable)
		return
	}

	if isWebSocketUpgrade(r) {
		h.proxyWebSocket(w, r, containerIP)
		return
	}

	target, _ := url.Parse(fmt.Sprintf("http://%s:6080", containerIP))
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *nethttp.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		originalPath := req.URL.Path
		if idx := strings.Index(originalPath, "/vnc/"); idx != -1 {
			req.URL.Path = "/" + originalPath[idx+5:]
		} else if strings.HasSuffix(originalPath, "/vnc") {
			req.URL.Path = "/"
		}
	}
	proxy.ErrorHandler = func(rw nethttp.ResponseWriter, req *nethttp.Request, err error) {
		h.logger.Error("vnc proxy error", "error", err)
		rw.WriteHeader(nethttp.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

func (h *AdminVNCProxyHandler) proxyWebSocket(w nethttp.ResponseWriter, r *nethttp.Request, containerIP string) {
	targetAddr := fmt.Sprintf("%s:6080", containerIP)

	targetConn, err := net.DialTimeout("tcp", targetAddr, 5*time.Second)
	if err != nil {
		h.logger.Error("vnc websocket dial failed", "addr", targetAddr, "error", err)
		nethttp.Error(w, "cannot connect to VNC", nethttp.StatusBadGateway)
		return
	}
	defer targetConn.Close()

	hijacker, ok := w.(nethttp.Hijacker)
	if !ok {
		nethttp.Error(w, "websocket hijack not supported", nethttp.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		nethttp.Error(w, "websocket hijack failed", nethttp.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// 重写路径：去掉 /v1/admin/hosts/{id}/vnc 前缀
	path := r.URL.Path
	if idx := strings.Index(path, "/vnc/"); idx != -1 {
		path = "/" + path[idx+5:]
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}

	// 重写 Host 头指向容器
	r.Header.Set("Host", targetAddr)
	r.Host = targetAddr

	reqLine := fmt.Sprintf("%s %s %s\r\n", r.Method, path, r.Proto)
	targetConn.Write([]byte(reqLine))
	r.Header.Write(targetConn)
	targetConn.Write([]byte("\r\n"))

	// 如果客户端 buffer 里有未读数据，先写过去
	if clientBuf.Reader.Buffered() > 0 {
		buffered := make([]byte, clientBuf.Reader.Buffered())
		clientBuf.Read(buffered)
		targetConn.Write(buffered)
	}

	done := make(chan struct{}, 2)
	go func() { io.Copy(targetConn, clientConn); done <- struct{}{} }()
	go func() { io.Copy(clientConn, targetConn); done <- struct{}{} }()
	<-done
}

func isWebSocketUpgrade(r *nethttp.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func getContainerIP(ctx context.Context, containerName string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", containerName)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect: %w", err)
	}
	ips := strings.Fields(strings.TrimSpace(string(out)))
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP found for container %s", containerName)
	}
	// Use the last IP — typically the isolated network, not the default bridge
	return ips[len(ips)-1], nil
}
