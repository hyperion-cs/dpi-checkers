package checkers

import (
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"
)

// TODO (options):
// - Try to extract sni/host from cert
// - Set proto (http/https) via TlsV
// - Set port
// - Set random sni/host (?)
// - Skip tcp1620 check (alive only)
type WebhostOpt struct {
	Ip netip.Addr
}

type WebhostAttr struct {
	Ip       netip.Addr
	Port     int
	TlsV     uint16 // plain http if 0
	Sni      string
	HttpHost string
	Alive    bool
	Tcp1620  bool
}

type webhostHttpReq struct {
	method string
	host   string
	body   []byte
}

var (
	ErrWebhostConn         = errors.New("tcp connection error")
	ErrWebhostTlsHandshake = errors.New("tls handshake error")
	ErrWebhostReq          = errors.New("request error")
	ErrWebhostResp         = errors.New("response error")
)

func WebhostSingle(opt WebhostOpt) (WebhostAttr, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	addr := net.JoinHostPort(opt.Ip.String(), "443")

	tcpConn, err := d.Dial("tcp", addr)
	if err != nil {
		return WebhostAttr{}, ErrWebhostConn
	}
	defer tcpConn.Close()

	sni := "" // without sni
	tlsConn := tls.Client(tcpConn, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
	})
	defer tlsConn.Close()

	tlsConn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		return WebhostAttr{}, ErrWebhostTlsHandshake
	}
	tlsConn.SetDeadline(time.Time{})

	req := prepareHttpReq(webhostHttpReq{method: "HEAD"})
	tcpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := tlsConn.Write(req); err != nil {
		return WebhostAttr{}, ErrWebhostReq
	}
	tcpConn.SetWriteDeadline(time.Time{})

	err = readIntoVacuum(tlsConn, 5*time.Second)
	if err != nil {
		return WebhostAttr{}, ErrWebhostResp
	}

	return WebhostAttr{
		Ip:       opt.Ip,
		Port:     443,
		TlsV:     tlsConn.ConnectionState().Version,
		Sni:      sni,
		HttpHost: "",
		Alive:    true,
		Tcp1620:  false,
	}, nil
}

func readIntoVacuum(tlsConn *tls.Conn, timeout time.Duration) error {
	tlsConn.SetReadDeadline(time.Now().Add(timeout))
	defer tlsConn.SetReadDeadline(time.Time{})
	if _, err := io.ReadAll(tlsConn); err != nil {
		return err
	}
	return nil
}

func prepareHttpReq(opt webhostHttpReq) []byte {
	reqStr := opt.method + " / HTTP/1.1\r\n" +
		"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36\r\n" +
		"Accept: */*\r\n"
	if len(opt.host) > 0 {
		reqStr += "Host: " + opt.host
	}
	if opt.body != nil {
		reqStr += fmt.Sprintf("Content-Length: %d", len(opt.body))
	}
	reqStr += "Connection: close\r\n\r\n"
	req := make([]byte, len(reqStr)+len(opt.body))
	req = append(req, []byte(reqStr)...)
	req = append(req, opt.body...)
	return req
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}
