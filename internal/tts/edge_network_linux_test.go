//go:build linux

package tts

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func localEdgeCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "TTS test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"speech.platform.bing.com"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := tls.Certificate{Certificate: [][]byte{leafDER, caDER}, PrivateKey: key}
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return certificate, caPath
}

// This exercises the fixed upstream Stream implementation, including its blocked
// post-SSML ReadMessage, in a real subprocess with normal certificate validation.
func TestEdgeRealStreamCancellationAndEscaping(t *testing.T) {
	certificate, caPath := localEdgeCertificate(t)
	ssml := make(chan string, 1)
	closed := make(chan struct{})
	serverErrors := make(chan error, 4)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			serverErrors <- err
			return
		}
		defer conn.Close()
		defer close(closed)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if strings.Contains(string(data), "Path:ssml\r\n") {
				_, body, found := strings.Cut(string(data), "\r\n\r\n")
				if !found {
					serverErrors <- errors.New("missing SSML body")
					return
				}
				ssml <- body
				// Deliberately never send turn.end. Stream must remain blocked until killed.
			}
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "speech.platform.bing.com:443" {
			http.Error(w, "unexpected CONNECT", http.StatusBadRequest)
			return
		}
		upstream, err := net.Dial("tcp", server.Listener.Addr().String())
		if err != nil {
			serverErrors <- err
			http.Error(w, "dial failed", http.StatusBadGateway)
			return
		}
		downstream, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			serverErrors <- err
			return
		}
		defer downstream.Close()
		defer upstream.Close()
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			serverErrors <- err
			return
		}
		if err := buffered.Flush(); err != nil {
			serverErrors <- err
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffered); upstream.Close(); close(done) }()
		_, _ = io.Copy(downstream, upstream)
		downstream.Close()
		<-done
	}))
	defer proxy.Close()
	text := "<voice name='x'>你好 & 再见</voice>\n\t结束"
	input, err := json.Marshal(edgeRequest{Text: text, Voice: defaultVoice, Proxy: proxy.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	env := append(withoutProxyEnv(fixtureEnv("edge")), "SSL_CERT_FILE="+caPath, "SSL_CERT_DIR="+t.TempDir())
	result := make(chan error, 1)
	executable := testExecutable(t)
	go func() { _, err := runHelper(ctx, executable, input, MaxAudioBytes, env); result <- err }()
	select {
	case body := <-ssml:
		var document struct {
			Voice []struct {
				Name    string `xml:"name,attr"`
				Prosody struct {
					Text   string     `xml:",chardata"`
					Voices []struct{} `xml:"voice"`
				} `xml:"prosody"`
			} `xml:"voice"`
		}
		if err := xml.Unmarshal([]byte(body), &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Voice) != 1 || document.Voice[0].Name != defaultVoice || document.Voice[0].Prosody.Text != text || len(document.Voice[0].Prosody.Voices) != 0 {
			t.Fatalf("SSML changed text or allowed injection: %s", body)
		}
	case err := <-result:
		t.Fatalf("Stream exited before SSML: %v", err)
	case err := <-serverErrors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("SSML not received")
	}
	started := time.Now()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("real Stream child was not reaped within two seconds")
	}
	if time.Since(started) >= 2*time.Second {
		t.Fatal("slow child cancellation")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream WebSocket remained open")
	}
}
