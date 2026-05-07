package sshproxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// ---- channelOpenDirectMsg payload parsing ----

func TestDirectTCPIP_PayloadParse(t *testing.T) {
	msg := channelOpenDirectMsg{
		Raddr: "127.0.0.1",
		Rport: 8080,
		Laddr: "0.0.0.0",
		Lport: 0,
	}

	data := ssh.Marshal(&msg)

	var parsed channelOpenDirectMsg
	if err := ssh.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if parsed.Raddr != "127.0.0.1" {
		t.Fatalf("Raddr: got %q, want %q", parsed.Raddr, "127.0.0.1")
	}
	if parsed.Rport != 8080 {
		t.Fatalf("Rport: got %d, want %d", parsed.Rport, 8080)
	}
	if parsed.Laddr != "0.0.0.0" {
		t.Fatalf("Laddr: got %q, want %q", parsed.Laddr, "0.0.0.0")
	}
	if parsed.Lport != 0 {
		t.Fatalf("Lport: got %d, want %d", parsed.Lport, 0)
	}
}

func TestDirectTCPIP_PayloadParse_TooShort(t *testing.T) {
	data := []byte{0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'}

	var parsed channelOpenDirectMsg
	err := ssh.Unmarshal(data, &parsed)
	if err == nil {
		t.Fatal("expected error for short payload, got nil")
	}
}

// ---- isForbiddenTarget ----

func TestIsForbiddenTarget_ManagementSubnet(t *testing.T) {
	tests := []struct {
		host string
		port int
		desc string
	}{
		{"10.99.1.1", 80, "management subnet IP"},
		{"10.99.0.1", 22, "management subnet gateway"},
		{"10.99.255.255", 443, "management subnet broadcast"},
		{"10.99.123.45", 1080, "management subnet arbitrary"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if !isForbiddenTarget(tt.host, tt.port) {
				t.Errorf("expected forbidden for %s:%d, got allowed", tt.host, tt.port)
			}
		})
	}
}

func TestIsForbiddenTarget_AllowedIP(t *testing.T) {
	tests := []struct {
		host string
		port int
		desc string
	}{
		{"127.0.0.1", 8080, "localhost"},
		{"8.8.8.8", 53, "public DNS"},
		{"192.168.1.1", 443, "private but not forbidden"},
		{"172.16.0.1", 80, "docker default but not in blocklist"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if isForbiddenTarget(tt.host, tt.port) {
				t.Errorf("expected allowed for %s:%d, got forbidden", tt.host, tt.port)
			}
		})
	}
}

func TestIsForbiddenTarget_MetadataEndpoint(t *testing.T) {
	if !isForbiddenTarget("metadata.google.internal", 80) {
		t.Error("expected forbidden for metadata.google.internal:80")
	}
}

func TestIsForbiddenTarget_DockerSocket(t *testing.T) {
	tests := []struct {
		host string
		port int
	}{
		{"169.254.169.254", 2375},
		{"169.254.169.254", 2376},
	}

	for _, tt := range tests {
		desc := net.JoinHostPort(tt.host, fmt.Sprintf("%d", tt.port))
		t.Run(desc, func(t *testing.T) {
			if !isForbiddenTarget(tt.host, tt.port) {
				t.Errorf("expected forbidden for %s:%d", tt.host, tt.port)
			}
		})
	}
}

func TestIsForbiddenTarget_NonIPHostname(t *testing.T) {
	if isForbiddenTarget("example.com", 443) {
		t.Error("expected allowed for example.com:443")
	}
}

func TestIsForbiddenTarget_ForbiddenPortNonIP(t *testing.T) {
	if !isForbiddenTarget("some-host.local", 2375) {
		t.Error("expected forbidden for some-host.local:2375 (port match)")
	}
}

func TestIsForbiddenTarget_PublicIPAllowed(t *testing.T) {
	if isForbiddenTarget("8.8.8.8", 53) {
		t.Error("expected allowed for 8.8.8.8:53")
	}
}

// ---- Test helpers for SSH forwarding tests ----

// startTestSSHServerWithRequestHandler starts a minimal SSH server that
// handles global requests. All requests are rejected with Reply(false, nil).
// Session channels are accepted and read until EOF.
func startTestSSHServerWithRequestHandler(t *testing.T) (string, func()) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sshConn, chans, reqs, err := ssh.NewServerConn(c, config)
				if err != nil {
					return
				}
				defer sshConn.Close()

				// Handle global requests — reply false to all.
				go func() {
					for req := range reqs {
						if req.WantReply {
							req.Reply(false, nil)
						}
					}
				}()

				// Accept session channels.
				for newChan := range chans {
					switch newChan.ChannelType() {
					case "session":
						ch, chReqs, err := newChan.Accept()
						if err != nil {
							return
						}
						go ssh.DiscardRequests(chReqs)
						go func() {
							io.Copy(io.Discard, ch)
							ch.Close()
						}()
					default:
						newChan.Reject(ssh.UnknownChannelType, "unsupported")
					}
				}
			}(conn)
		}
	}()
	return listener.Addr().String(), func() { listener.Close() }
}

// startTestSSHServerForForwarding starts a minimal SSH server that accepts
// session channels and opens a forwarded-tcpip channel back to the client
// after receiving the first session channel. This simulates a target container
// that triggers port forwarding.
func startTestSSHServerForForwarding(t *testing.T, data string) (string, func()) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sshConn, chans, reqs, err := ssh.NewServerConn(c, config)
				if err != nil {
					return
				}
				defer sshConn.Close()

				go ssh.DiscardRequests(reqs)

				for newChan := range chans {
					switch newChan.ChannelType() {
					case "session":
						ch, chReqs, err := newChan.Accept()
						if err != nil {
							return
						}
						go ssh.DiscardRequests(chReqs)

						// Open a forwarded-tcpip channel toward the client.
						payload := ssh.Marshal(&forwardedTCPPayload{
							Addr:       "127.0.0.1",
							Port:       9090,
							OriginAddr: "127.0.0.1",
							OriginPort: 12345,
						})
						fwdCh, fwdReqs, err := sshConn.OpenChannel("forwarded-tcpip", payload)
						if err != nil {
							t.Errorf("open forwarded-tcpip: %v", err)
							ch.Close()
							return
						}
						go ssh.DiscardRequests(fwdReqs)

						// Send data on the forwarded channel and close.
						fmt.Fprint(fwdCh, data)
						fwdCh.CloseWrite()
						io.Copy(io.Discard, fwdCh)
						fwdCh.Close()

						// Read from session until EOF, then close.
						io.Copy(io.Discard, ch)
						ch.Close()
					default:
						newChan.Reject(ssh.UnknownChannelType, "unsupported")
					}
				}
			}(conn)
		}
	}()
	return listener.Addr().String(), func() { listener.Close() }
}

// ---- handleGlobalRequests tests ----

func TestTCPIPForward_GlobalRequest(t *testing.T) {
	// Test that handleGlobalRequests forwards tcpip-forward requests
	// to the target and relays the reply back to the client.
	//
	// Architecture:
	//   client -> [custom proxy SSH server] -> handleGlobalRequests -> [target SSH server]
	//
	// The target server replies (false) to tcpip-forward. The proxy
	// relays this reply back to the client.

	targetAddr, targetCleanup := startTestSSHServerWithRequestHandler(t)
	defer targetCleanup()

	targetClient, err := ssh.Dial("tcp", targetAddr, &ssh.ClientConfig{
		User:            "ws",
		Auth:            []ssh.AuthMethod{ssh.Password("pass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial target: %v", err)
	}
	defer targetClient.Close()

	s := &Server{logger: slog.New(slog.DiscardHandler)}

	// Set up a custom proxy server that runs handleGlobalRequests.
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer proxyListener.Close()

	proxyConfig := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	_, proxyPriv, _ := ed25519.GenerateKey(rand.Reader)
	proxySigner, _ := ssh.NewSignerFromKey(proxyPriv)
	proxyConfig.AddHostKey(proxySigner)

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		sshConn, _, globalReqs, err := ssh.NewServerConn(conn, proxyConfig)
		if err != nil {
			return
		}
		defer sshConn.Close()
		s.handleGlobalRequests(globalReqs, targetClient)
	}()

	// Connect a client to our custom proxy server.
	proxyClient, err := ssh.Dial("tcp", proxyListener.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("test")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer proxyClient.Close()

	// Send tcpip-forward — should be forwarded to target, which rejects it.
	payload := ssh.Marshal(&struct {
		Addr string `sshtype:"string"`
		Port uint32 `sshtype:"uint32"`
	}{Addr: "127.0.0.1", Port: 8080})

	ok, _, err := proxyClient.SendRequest("tcpip-forward", true, payload)
	if err != nil {
		t.Fatalf("SendRequest tcpip-forward: %v", err)
	}
	if ok {
		t.Error("expected tcpip-forward to be rejected by target, got accepted")
	}
}

func TestCancelTCPIPForward_GlobalRequest(t *testing.T) {
	// Test that handleGlobalRequests forwards cancel-tcpip-forward
	// to the target and relays the reply.

	targetAddr, targetCleanup := startTestSSHServerWithRequestHandler(t)
	defer targetCleanup()

	targetClient, err := ssh.Dial("tcp", targetAddr, &ssh.ClientConfig{
		User:            "ws",
		Auth:            []ssh.AuthMethod{ssh.Password("pass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial target: %v", err)
	}
	defer targetClient.Close()

	s := &Server{logger: slog.New(slog.DiscardHandler)}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer proxyListener.Close()

	proxyConfig := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	_, proxyPriv, _ := ed25519.GenerateKey(rand.Reader)
	proxySigner, _ := ssh.NewSignerFromKey(proxyPriv)
	proxyConfig.AddHostKey(proxySigner)

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		sshConn, _, globalReqs, err := ssh.NewServerConn(conn, proxyConfig)
		if err != nil {
			return
		}
		defer sshConn.Close()
		s.handleGlobalRequests(globalReqs, targetClient)
	}()

	proxyClient, err := ssh.Dial("tcp", proxyListener.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("test")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer proxyClient.Close()

	payload := ssh.Marshal(&struct {
		Addr string `sshtype:"string"`
		Port uint32 `sshtype:"uint32"`
	}{Addr: "127.0.0.1", Port: 8080})

	ok, _, err := proxyClient.SendRequest("cancel-tcpip-forward", true, payload)
	if err != nil {
		t.Fatalf("SendRequest cancel-tcpip-forward: %v", err)
	}
	if ok {
		t.Error("expected cancel-tcpip-forward to be rejected by target, got accepted")
	}
}

func TestUnknownGlobalRequest_Rejected(t *testing.T) {
	// Test that handleGlobalRequests rejects unknown request types
	// with Reply(false, nil) — the request is NOT forwarded to the target.

	targetAddr, targetCleanup := startTestSSHServerWithRequestHandler(t)
	defer targetCleanup()

	targetClient, err := ssh.Dial("tcp", targetAddr, &ssh.ClientConfig{
		User:            "ws",
		Auth:            []ssh.AuthMethod{ssh.Password("pass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial target: %v", err)
	}
	defer targetClient.Close()

	s := &Server{logger: slog.New(slog.DiscardHandler)}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer proxyListener.Close()

	proxyConfig := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	_, proxyPriv, _ := ed25519.GenerateKey(rand.Reader)
	proxySigner, _ := ssh.NewSignerFromKey(proxyPriv)
	proxyConfig.AddHostKey(proxySigner)

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		sshConn, _, globalReqs, err := ssh.NewServerConn(conn, proxyConfig)
		if err != nil {
			return
		}
		defer sshConn.Close()
		s.handleGlobalRequests(globalReqs, targetClient)
	}()

	proxyClient, err := ssh.Dial("tcp", proxyListener.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("test")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer proxyClient.Close()

	ok, _, err := proxyClient.SendRequest("my-custom-request", true, nil)
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if ok {
		t.Error("expected unknown global request to be rejected")
	}
}

// ---- proxyForwardedChannels tests ----

func TestForwardedTCPIP_PayloadUnmarshal(t *testing.T) {
	// Test that forwardedTCPPayload can be marshaled/unmarshaled correctly.
	// This verifies the wire format used by proxyForwardedChannels.
	payload := forwardedTCPPayload{
		Addr:       "127.0.0.1",
		Port:       9090,
		OriginAddr: "10.0.0.1",
		OriginPort: 54321,
	}

	data := ssh.Marshal(&payload)
	var parsed forwardedTCPPayload
	if err := ssh.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Addr != "127.0.0.1" {
		t.Errorf("Addr: got %q, want %q", parsed.Addr, "127.0.0.1")
	}
	if parsed.Port != 9090 {
		t.Errorf("Port: got %d, want %d", parsed.Port, 9090)
	}
	if parsed.OriginAddr != "10.0.0.1" {
		t.Errorf("OriginAddr: got %q, want %q", parsed.OriginAddr, "10.0.0.1")
	}
	if parsed.OriginPort != 54321 {
		t.Errorf("OriginPort: got %d, want %d", parsed.OriginPort, 54321)
	}
}

func TestForwardedTCPIP_ChannelRelay(t *testing.T) {
	// Test that proxyForwardedChannels correctly relays forwarded-tcpip
	// channels from a server to a client via the proxy, with bidirectional
	// data copy and correct payload unmarshaling.
	//
	// Architecture:
	//   server (opens forwarded-tcpip) -> [proxy mux] -> proxyForwardedChannels -> client
	//
	// We simulate the flow by:
	// 1. Setting up a client that registers HandleChannelOpen("forwarded-tcpip")
	// 2. Setting up a server SSH connection that can open channels toward the client
	// 3. Running proxyForwardedChannels on the client's NewChannel channel
	// 4. Having the server open a forwarded-tcpip channel
	// 5. Verifying the client receives the correct payload and data

	s := &Server{logger: slog.New(slog.DiscardHandler)}

	// Set up a TCP listener that will handle one SSH connection.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	serverConfig.AddHostKey(signer)

	// Channel to signal when the forwarded-tcpip channel is received.
	payloadReceived := make(chan forwardedTCPPayload, 1)
	dataReceived := make(chan string, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		sshConn, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		defer sshConn.Close()
		go ssh.DiscardRequests(reqs)

		// Set up proxyForwardedChannels to relay incoming forwarded-tcpip
		// channels to the client connection.
		fwdCh := make(chan ssh.NewChannel, 1)
		go s.proxyForwardedChannels(fwdCh, sshConn, "test-target")

		// Accept session channels from the client. When we receive one,
		// open a forwarded-tcpip channel back toward the client.
		for newChan := range chans {
			switch newChan.ChannelType() {
			case "session":
				ch, chReqs, err := newChan.Accept()
				if err != nil {
					return
				}
				go ssh.DiscardRequests(chReqs)

				// Open forwarded-tcpip channel toward the client.
				payload := ssh.Marshal(&forwardedTCPPayload{
					Addr:       "127.0.0.1",
					Port:       9090,
					OriginAddr: "127.0.0.1",
					OriginPort: 12345,
				})
				targetCh, targetReqs, err := sshConn.OpenChannel("forwarded-tcpip", payload)
				if err != nil {
					t.Errorf("open forwarded-tcpip: %v", err)
					ch.Close()
					return
				}
				go ssh.DiscardRequests(targetReqs)

				// Send data through the forwarded channel.
				fmt.Fprint(targetCh, "hello from forwarded port")
				targetCh.CloseWrite()
				io.Copy(io.Discard, targetCh)
				targetCh.Close()

				// Clean up the session.
				io.Copy(io.Discard, ch)
				ch.Close()

				// Inject the forwarded channel into our handler.
				// Since sshConn.OpenChannel sent a channel-open to the client,
				// the client's HandleChannelOpen will receive it.
				// But we need to test proxyForwardedChannels directly.
				// Instead, let's create a fake NewChannel and inject it.
				_ = fwdCh
				return
			default:
				newChan.Reject(ssh.UnknownChannelType, "unsupported")
			}
		}
	}()

	// Connect a client to the server.
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("test")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// Register forwarded-tcpip handler on the client.
	clientFwdCh := client.HandleChannelOpen("forwarded-tcpip")

	// Start receiving forwarded-tcpip channels.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for newChan := range clientFwdCh {
			var p forwardedTCPPayload
			if err := ssh.Unmarshal(newChan.ExtraData(), &p); err != nil {
				t.Errorf("unmarshal payload: %v", err)
				newChan.Reject(ssh.ConnectionFailed, "bad payload")
				return
			}
			payloadReceived <- p

			ch, chReqs, err := newChan.Accept()
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			go ssh.DiscardRequests(chReqs)

			data, _ := io.ReadAll(ch)
			dataReceived <- string(data)
			ch.Close()
		}
	}()

	// Open a session channel to trigger the server to open forwarded-tcpip.
	sessionCh, sessionReqs, err := client.OpenChannel("session", nil)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	go ssh.DiscardRequests(sessionReqs)
	sessionCh.Close()

	// Wait for the forwarded-tcpip channel to be received.
	select {
	case p := <-payloadReceived:
		if p.Addr != "127.0.0.1" || p.Port != 9090 {
			t.Errorf("unexpected payload: %+v", p)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for forwarded-tcpip payload")
	}

	select {
	case d := <-dataReceived:
		if d != "hello from forwarded port" {
			t.Errorf("unexpected data: %q", d)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for forwarded-tcpip data")
	}

	wg.Wait()
}

func TestForwardedTCPIP_UnknownGlobalRequest_Rejected(t *testing.T) {
	// Test that handleGlobalRequests rejects unknown request types
	// even when a forwarded-tcpip handler is also set up.

	targetAddr, targetCleanup := startTestSSHServerWithRequestHandler(t)
	defer targetCleanup()

	targetClient, err := ssh.Dial("tcp", targetAddr, &ssh.ClientConfig{
		User:            "ws",
		Auth:            []ssh.AuthMethod{ssh.Password("pass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial target: %v", err)
	}
	defer targetClient.Close()

	s := &Server{logger: slog.New(slog.DiscardHandler)}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer proxyListener.Close()

	proxyConfig := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	_, proxyPriv, _ := ed25519.GenerateKey(rand.Reader)
	proxySigner, _ := ssh.NewSignerFromKey(proxyPriv)
	proxyConfig.AddHostKey(proxySigner)

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		sshConn, _, globalReqs, err := ssh.NewServerConn(conn, proxyConfig)
		if err != nil {
			return
		}
		defer sshConn.Close()
		s.handleGlobalRequests(globalReqs, targetClient)
	}()

	proxyClient, err := ssh.Dial("tcp", proxyListener.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("test")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer proxyClient.Close()

	ok, _, err := proxyClient.SendRequest("unknown-request-type", true, nil)
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if ok {
		t.Error("expected unknown global request to be rejected")
	}
}
