package checkers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"dpich/config"
	"dpich/inetlookup"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	tls "github.com/refraction-networking/utls"
)

// TODO (options):
// - Set proto (http/https)
// - Set tlsV
// - Set port
// - Set random sni/host (?)
// - Try to extract sni/host from cert
// - Skip tcp1620 check (alive only)
type WebhostSingleOpt struct {
	Ip           netip.Addr
	Port         int
	KeyLogWriter io.Writer
	Sni          string
	Host         string
}

type WebhostSingleResult struct {
	IpInfo   inetlookup.IpInfo
	Port     int
	TlsV     uint16
	Sni      string
	HttpHost string
	Alive    error
	Tcp1620  error
}

type webhostHttpReq struct {
	method string
	host   string
	body   []byte
}

var (
	ErrWebhostTcpConnReset        = errors.New("tcp: connection reset")
	ErrWebhostTcpConnTimeout      = errors.New("tcp: connection timeout")
	ErrWebhostTcpWriteTimeout     = errors.New("tcp: write timeout")
	ErrWebhostTcpReadTimeout      = errors.New("tcp: read timeout")
	ErrWebhostTlsHandshakeTimeout = errors.New("tls: handshake timeout")
	ErrWebhostTlsHandshakeFail    = errors.New("tls: handshake failure")
	ErrWebhostTlsBadRecordMac     = errors.New("tls: bad record MAC")
	ErrWebhostInternal            = errors.New("check: internal error")
	ErrWebhostSkip                = errors.New("check: skip")
)

func WebhostSingle(opt WebhostSingleOpt) WebhostSingleResult {
	if opt.KeyLogWriter == nil {
		opt.KeyLogWriter = io.Discard
	}

	ipinfo := inetlookup.Default().IpInfo(opt.Ip)
	res := WebhostSingleResult{IpInfo: ipinfo, Port: opt.Port}

	// alive check
	tlsConn, err := getHandshakedUTlsConn(opt, opt.KeyLogWriter)
	if err != nil {
		res.Alive = err
		res.Tcp1620 = ErrWebhostSkip
		return res
	}
	res.TlsV = tlsConn.ConnectionState().Version
	if err = webhostAliveCheck(opt, tlsConn); err != nil {
		res.Alive = err
		res.Tcp1620 = ErrWebhostSkip
		return res
	}

	// tcp16-20 check
	tlsConn, err = getHandshakedUTlsConn(opt, opt.KeyLogWriter)
	if err != nil {
		res.Tcp1620 = err
		return res
	}
	if err = webhostTcp1620check(opt, tlsConn); err != nil {
		res.Tcp1620 = err
	}

	return res
}

func setUTlsAlpn(spec *tls.ClientHelloSpec, protos []string) {
	for i := range spec.Extensions {
		if alpn, ok := spec.Extensions[i].(*tls.ALPNExtension); ok {
			alpn.AlpnProtocols = protos
			return
		}
	}
	spec.Extensions = append(spec.Extensions, &tls.ALPNExtension{AlpnProtocols: protos})
}

func getHandshakedUTlsConn(opt WebhostSingleOpt, keyLogWriter io.Writer) (*tls.UConn, error) {
	cfg := config.Get().Checkers.Webhost
	tcpDialer := net.Dialer{Timeout: cfg.TcpConnTimeout}
	addr := net.JoinHostPort(opt.Ip.String(), strconv.Itoa(opt.Port))

	tcpConn, err := tcpDialer.Dial("tcp", addr)
	if err != nil {
		if isTimeout(err) {
			return nil, ErrWebhostTcpConnTimeout
		}
		log.Println("getHandshakedUTlsConn/Dial", err)
		return nil, ErrWebhostInternal
	}

	rawTcpConn := tcpConn.(*net.TCPConn)
	rawTcpConn.SetWriteBuffer(cfg.TcpWriteBuf)
	rawTcpConn.SetReadBuffer(cfg.TcpReadBuf)

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		KeyLogWriter:       keyLogWriter,
	}

	if opt.Sni != "" {
		tlsConf.ServerName = opt.Sni
	}

	tlsConn := tls.UClient(tcpConn, tlsConf, tls.HelloCustom)

	// chrome fingerprint originally contains ALPN for h2
	chromeSpec, _ := tls.UTLSIdToSpec(tls.HelloChrome_133)
	setUTlsAlpn(&chromeSpec, []string{"http/1.1"})
	tlsConn.ApplyPreset(&chromeSpec)

	tlsConn.SetDeadline(time.Now().Add(cfg.TlsHandshakeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		if isTimeout(err) {
			return nil, ErrWebhostTlsHandshakeTimeout
		}
		// https://go.dev/src/crypto/tls/alert.go
		if strings.Contains(err.Error(), "handshake failure") {
			return nil, ErrWebhostTlsHandshakeFail
		}
		if strings.Contains(err.Error(), "connection reset") {
			return nil, ErrWebhostTcpConnReset
		}

		log.Println("getHandshakedUTlsConn/Handshake", err)
		return nil, ErrWebhostInternal
	}
	tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}

func tlsReadHttpHeaders(tlsConn *tls.UConn) ([]byte, error) {
	cfg := config.Get().Checkers.Webhost
	tlsConn.SetReadDeadline(time.Now().Add(cfg.TcpReadTimeout))
	defer tlsConn.SetReadDeadline(time.Time{})

	br := bufio.NewReader(tlsConn)
	var buf []byte
	needle := []byte("\r\n\r\n")

	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			if isTimeout(err) {
				return nil, ErrWebhostTcpReadTimeout
			}
			if strings.Contains(err.Error(), "connection reset") {
				return nil, ErrWebhostTcpConnReset
			}
			// https://go.dev/src/crypto/tls/alert.go
			if strings.Contains(err.Error(), "bad record MAC") {
				return buf, ErrWebhostTlsBadRecordMac
			}
			log.Println("tlsReadAll", err)
			return nil, ErrWebhostInternal
		}

		buf = append(buf, line...)
		if bytes.HasSuffix(buf, needle) {
			return buf, nil
		}
	}
}

func tlsWriteAll(tlsConn *tls.UConn, data []byte) error {
	cfg := config.Get().Checkers.Webhost
	tlsConn.SetWriteDeadline(time.Now().Add(cfg.TcpWriteTimeout))
	defer tlsConn.SetWriteDeadline(time.Time{})
	if _, err := tlsConn.Write(data); err != nil {
		if isTimeout(err) {
			return ErrWebhostTcpWriteTimeout
		}
		log.Println("tlsWriteAll", err)
		return ErrWebhostInternal
	}
	return nil
}

func webhostAliveCheck(opt WebhostSingleOpt, tlsConn *tls.UConn) error {
	defer tlsConn.Close()
	req := prepareHttpReq(webhostHttpReq{method: "HEAD", host: opt.Host})
	if err := tlsWriteAll(tlsConn, req); err != nil {
		return err
	}
	if _, err := tlsReadHttpHeaders(tlsConn); err != nil {
		return err
	}
	return nil
}

func webhostTcp1620check(opt WebhostSingleOpt, tlsConn *tls.UConn) error {
	defer tlsConn.Close()
	cfg := config.Get().Checkers.Webhost
	body, _ := randomBytes(cfg.Tcp1620nBytes)
	req := prepareHttpReq(webhostHttpReq{method: "POST", host: opt.Host, body: body})
	if err := tlsWriteAll(tlsConn, req); err != nil {
		return err
	}
	if _, err := tlsReadHttpHeaders(tlsConn); err != nil {
		return err
	}
	return nil
}

func prepareHttpReq(opt webhostHttpReq) []byte {
	cfg := config.Get().Checkers.Webhost
	reqStr := opt.method + " / HTTP/1.1\r\n"
	for k, v := range cfg.HttpStaticHeaders {
		reqStr += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	if len(opt.host) > 0 {
		reqStr += fmt.Sprintf("Host: %s\r\n", opt.host)
	}
	if opt.body != nil {
		reqStr += fmt.Sprintf("Content-Length: %d\r\n", len(opt.body))
	}
	reqStr += "Connection: close\r\n\r\n"

	req := make([]byte, 0, len(reqStr)+len(opt.body))
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

func isTimeout(err error) bool {
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, os.ErrDeadlineExceeded) {
			return true
		} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return true
		}
	}
	return false
}
