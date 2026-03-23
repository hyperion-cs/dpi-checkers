package checkers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/netip"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/httputil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"

	tls "github.com/refraction-networking/utls"
)

type WebhostSingleOpt struct {
	Ip             netip.Addr
	Port           int
	KeyLogWriter   io.Writer
	Sni            string
	Host           string
	Tcp1620skip    bool
	RandomHostname bool
}

type WebhostSingleResult struct {
	IpInfo  inetlookup.IpInfo
	Port    int
	TlsV    uint16
	Sni     string
	Host    string
	Alive   error
	Tcp1620 error
}

type webhostHttpReq struct {
	method    string
	host      string
	body      []byte
	keepalive bool
}

var (
	ErrWebhostInternal = errors.New("check: internal error")
	ErrWebhostSkip     = errors.New("check: skip")
)

const RANDOM_HOSTNAME_ALPHABET = "abcdefghijklmnopqrstuvwxyz0123456789"
const RANDOM_HOSTNAME_LEN = 12

func WebhostSingle(opt WebhostSingleOpt) WebhostSingleResult {
	if opt.KeyLogWriter == nil {
		opt.KeyLogWriter = io.Discard
	}

	if opt.RandomHostname {
		rndHostname, _ := randomHostname()
		opt.Sni = rndHostname
		opt.Host = rndHostname
	}

	ipinfo := inetlookup.Default().IpInfo(opt.Ip)
	res := WebhostSingleResult{
		IpInfo: ipinfo,
		Port:   opt.Port,
		Sni:    opt.Sni,
		Host:   opt.Host,
	}

	cfg := config.Get().Checkers.Webhost
	tlsConnOpt := httputil.TlsConnOpt{
		Ip:                  opt.Ip,
		Port:                opt.Port,
		Sni:                 opt.Sni,
		TcpConnTimeout:      cfg.TcpConnTimeout,
		TcpWriteBuf:         cfg.TcpWriteBuf,
		TcpReadBuf:          cfg.TcpReadBuf,
		TlsHandshakeTimeout: cfg.TlsHandshakeTimeout,
		KeyLogWriter:        opt.KeyLogWriter,
	}

	tlsConn, err := httputil.GetHandshakedUTlsConn(tlsConnOpt)
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

	if opt.Tcp1620skip {
		res.Tcp1620 = ErrWebhostSkip
		return res
	}

	tlsConn, err = httputil.GetHandshakedUTlsConn(tlsConnOpt)
	if err != nil {
		res.Tcp1620 = err
		return res
	}
	if err = webhostTcp1620check(opt, tlsConn); err != nil {
		res.Tcp1620 = err
	}

	return res
}

func webhostAliveCheck(opt WebhostSingleOpt, tlsConn *tls.UConn) error {
	defer tlsConn.Close()
	cfg := config.Get().Checkers.Webhost
	req := prepareHttpReq(webhostHttpReq{method: "HEAD", host: opt.Host})
	if err := httputil.TlsWriteAll(tlsConn, req, cfg.TcpWriteTimeout); err != nil {
		return err
	}
	if _, err := httputil.TlsReadHttpHeaders(tlsConn, cfg.TcpReadTimeout); err != nil {
		return err
	}
	return nil
}

func webhostTcp1620check(opt WebhostSingleOpt, tlsConn *tls.UConn) error {
	defer tlsConn.Close()
	cfg := config.Get().Checkers.Webhost
	body, _ := randomBytes(cfg.Tcp1620nBytes)

	// keep-alive increases the chance that we will be able to push enough data into the connection
	req := prepareHttpReq(webhostHttpReq{method: "POST", host: opt.Host, body: body, keepalive: true})
	if err := httputil.TlsWriteAll(tlsConn, req, cfg.TcpWriteTimeout); err != nil {
		return err
	}
	if _, err := httputil.TlsReadHttpHeaders(tlsConn, cfg.TcpReadTimeout); err != nil {
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

	conn := "close"
	if opt.keepalive {
		conn = "keep-alive"
	}
	reqStr += fmt.Sprintf("Connection: %s\r\n\r\n", conn)

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

func randomHostname() (string, error) {
	const tld = "com"
	b := make([]byte, RANDOM_HOSTNAME_LEN)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	const rha = RANDOM_HOSTNAME_ALPHABET
	for i := range b {
		b[i] = rha[int(b[i])%len(rha)]
	}

	return fmt.Sprintf("%s.%s", string(b), tld), nil
}
