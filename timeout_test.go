package httphack

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandshakeTimeout(t *testing.T) {
	defer http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	tunnel := newTunnel(host(srv.URL))
	defer tunnel.Close()

	tunnel.LockRead()
	defer tunnel.UnlockRead()

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(n, addr string) (net.Conn, error) {
				conn, err := net.Dial(n, addr)
				if err != nil {
					return nil, err
				}
				return NewTimeoutConn(conn, 100 * time.Millisecond), nil
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	_, err := client.Get("https://" + tunnel.Addr())
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("want a timeout error, got: %v", err)
	}
}

func TestIdleConnTimeout(t *testing.T) {
	defer http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "keep-alive")
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	tunnel := newTunnel(host(srv.URL))
	defer tunnel.Close()

	dials := 0

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(n, addr string) (net.Conn, error) {
				dials++
				conn, err := net.Dial(n, addr)
				if err != nil {
					return nil, err
				}
				return NewTimeoutConn(conn, 500 * time.Millisecond), nil
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", "https://" + tunnel.Addr(), nil)
	if err != nil {
		t.Fatalf("cannot build request: %v", err)
	}
	req.Header = http.Header{
		"Connection": []string{"keep-alive"},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected GET error: %v", err)
	}
	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		t.Fatalf("unexpected error reading response: %v", err)
	}
	resp.Body.Close()

	time.Sleep(1 * time.Second)
	tunnel.LockRead()
	defer tunnel.UnlockRead()

	_, err = client.Get("https://" + tunnel.Addr())
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("want a timeout error, got: %v", err)
	}

	if dials != 1 {
		t.Fatalf("expected one dial, got %d", dials)
	}
}

func TestBodyTimeout(t *testing.T) {
	defer http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(n, addr string) (net.Conn, error) {
				conn, err := net.Dial(n, addr)
				if err != nil {
					return nil, err
				}
				return NewTimeoutConn(conn, 100 * time.Millisecond), nil
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	_, err := client.Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("want a timeout error, got: %v", err)
	}
}
