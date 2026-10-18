package hosting

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNativeWebSocketAndCrashRecovery(t *testing.T) {
	a := testApp(t)
	provisionTestRuntime(t, a, "caddy")
	provisionTestRuntime(t, a, "node24")
	port, e := freePort()
	if e != nil {
		t.Fatal(e)
	}
	s := a.settings()
	s.CaddyPort = port
	_ = a.store.setSetting("hosting", s)
	p := testProject(t, a)
	p.Adapter, p.Trusted, p.Start = "node", true, []string{"node", "server.js"}
	_ = a.store.saveProject(p)
	source := `const http=require('node:http'),crypto=require('node:crypto');
const server=http.createServer((req,res)=>{res.end('WebSocket fixture');if(req.url==='/crash')setTimeout(()=>process.exit(7),50)});
server.on('upgrade',(req,socket)=>{const accept=crypto.createHash('sha1').update(req.headers['sec-websocket-key']+'258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');socket.write('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: '+accept+'\r\n\r\n');socket.on('data',data=>{if(data.length<6)return;const n=data[1]&127,mask=data.subarray(2,6),body=Buffer.from(data.subarray(6,6+n));for(let i=0;i<body.length;i++)body[i]^=mask[i%4];socket.write(Buffer.concat([Buffer.from([0x81,body.length]),body]));});});
server.listen(Number(process.env.PORT),'127.0.0.1');`
	d := uploadFixture(t, a, p, "production", map[string]string{"server.js": source})
	if d.State != "published" {
		t.Fatal(d.State, d.Error)
	}
	conn, e := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "GET /socket HTTP/1.1\r\nHost: %s.localhost\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", p.Slug)
	reader := bufio.NewReader(conn)
	response, e := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if e != nil || response.StatusCode != 101 {
		t.Fatal("Caddy upgrade failed", response, e)
	}
	if response.Header.Get("Sec-WebSocket-Accept") != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatal("WebSocket handshake was changed")
	}
	body := []byte("actual frame through Caddy")
	frame := []byte{0x81, 0x80 | byte(len(body)), 11, 22, 33, 44}
	for i, b := range body {
		frame = append(frame, b^frame[2+i%4])
	}
	if _, e = conn.Write(frame); e != nil {
		t.Fatal(e)
	}
	header := make([]byte, 2)
	if _, e = io.ReadFull(reader, header); e != nil {
		t.Fatal(e)
	}
	if header[0] != 0x81 || int(header[1]) != len(body) {
		t.Fatal("Unexpected WebSocket frame", header)
	}
	echo := make([]byte, len(body))
	if _, e = io.ReadFull(reader, echo); e != nil || string(echo) != string(body) {
		t.Fatal("WebSocket echo failed", string(echo), e)
	}
	conn.Close()
	a.processMu.Lock()
	proc := a.processes[d.ID]
	a.processMu.Unlock()
	proc.mu.Lock()
	originalPID := proc.pid
	proc.mu.Unlock()
	request, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/crash", port), nil)
	request.Host = p.Slug + ".localhost"
	client := &http.Client{Timeout: 3 * time.Second}
	r, e := client.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		proc.mu.Lock()
		restarted := proc.pid != originalPID && proc.restarts >= 1 && proc.state == "running"
		proc.mu.Unlock()
		if restarted {
			if code, body := getSite(t, port, p.Slug+".localhost"); code == 200 && strings.Contains(body, "WebSocket fixture") {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Crashed Node process did not restart with working traffic", proc.summary())
}
