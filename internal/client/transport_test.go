package client

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	utls "github.com/refraction-networking/utls"
)

// withHelloID swaps the package-level ClientHello id for one test. Like the
// other process globals in this package, tests using it must not run parallel.
func withHelloID(t *testing.T, id utls.ClientHelloID) {
	t.Helper()
	orig := chromeHelloID
	chromeHelloID = id
	t.Cleanup(func() { chromeHelloID = orig })
}

// liveTCP returns an address that accepts connections and then does nothing, so
// a dial succeeds and the test can fail the step it actually targets.
func liveTCP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	return ln.Addr().String()
}

// deadTCP returns an address nothing is listening on.
func deadTCP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func okSpec() (utls.ClientHelloSpec, error) { return chromeSpecHTTP1() }

func serverName(host string) *utls.Config {
	h, _, _ := net.SplitHostPort(host)
	return &utls.Config{ServerName: h, InsecureSkipVerify: true} //nolint:gosec // test server uses a self-signed cert
}

// Every step of the dial has to close the raw connection it opened before
// returning, or a failing handshake leaks a socket per attempt.
func TestChromeDialFailurePaths(t *testing.T) {
	ctx := context.Background()

	t.Run("address without a port", func(t *testing.T) {
		if _, err := chromeDial(ctx, "tcp", "no-port", okSpec, serverName); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		if _, err := chromeDial(ctx, "tcp", deadTCP(t), okSpec, serverName); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("spec build fails", func(t *testing.T) {
		bad := func() (utls.ClientHelloSpec, error) { return utls.ClientHelloSpec{}, errors.New("no spec") }
		_, err := chromeDial(ctx, "tcp", liveTCP(t), bad, serverName)
		if err == nil || !strings.Contains(err.Error(), "no spec") {
			t.Fatalf("err = %v, want the spec failure", err)
		}
	})

	t.Run("preset rejected", func(t *testing.T) {
		greaseOnly := func() (utls.ClientHelloSpec, error) {
			return utls.ClientHelloSpec{Extensions: []utls.TLSExtension{
				&utls.SupportedVersionsExtension{Versions: []uint16{utls.GREASE_PLACEHOLDER}},
			}}, nil
		}
		_, err := chromeDial(ctx, "tcp", liveTCP(t), greaseOnly, serverName)
		if err == nil || !strings.Contains(err.Error(), "utls preset") {
			t.Fatalf("err = %v, want the preset failure", err)
		}
	})

	t.Run("handshake fails against a plaintext server", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer srv.Close()
		addr := strings.TrimPrefix(srv.URL, "http://")

		_, err := chromeDial(ctx, "tcp", addr, okSpec, serverName)
		if err == nil || !strings.Contains(err.Error(), "utls handshake") {
			t.Fatalf("err = %v, want the handshake failure", err)
		}
	})
}

// The happy path: a complete Chrome-fingerprinted handshake.
func TestChromeDialCompletesTheHandshake(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	conn, err := chromeDial(context.Background(), "tcp", addr, okSpec, serverName)
	if err != nil {
		t.Fatalf("chromeDial: %v", err)
	}
	defer conn.Close()

	u, ok := conn.(*utls.UConn)
	if !ok {
		t.Fatalf("conn is %T, want a *utls.UConn", conn)
	}
	if !u.ConnectionState().HandshakeComplete {
		t.Error("handshake did not complete")
	}
}

// An unknown ClientHello id must fail fast in New rather than at the first
// request, so the client can fall back to the stdlib transport.
func TestNewChromeTransportRejectsAnUnknownHelloID(t *testing.T) {
	withHelloID(t, utls.ClientHelloID{Client: "nope", Version: "0"})

	if _, err := newChromeTransport(); err == nil {
		t.Fatal("want an error for an unknown ClientHello id")
	}
}

// The production transport verifies certificates: pointed at a self-signed
// server it must refuse, which is also the only way to execute its dial closure.
func TestChromeTransportVerifiesServerCertificates(t *testing.T) {
	tr, err := newChromeTransport()
	if err != nil {
		t.Fatalf("newChromeTransport: %v", err)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	_, err = (&http.Client{Transport: tr}).Get(srv.URL)
	if err == nil {
		t.Fatal("want a certificate error against a self-signed server")
	}
	var ce *tls.CertificateVerificationError
	if !errors.As(err, &ce) && !strings.Contains(err.Error(), "certificate") {
		t.Errorf("err = %v, want a certificate verification failure", err)
	}
}
