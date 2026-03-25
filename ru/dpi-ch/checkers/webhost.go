package checkers

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	req, err := http.NewRequest("HEAD", "https://"+opt.Host, http.NoBody)
	if err != nil {
		return err
	}
	req.Close = true
	httputil.SetHeaders(&req.Header, cfg.HttpStaticHeaders)

	writeCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpWriteTimeout)
	defer cancel()
	if err := httputil.TlsWriteHttpRequest(writeCtx, tlsConn, req); err != nil {
		return err
	}

	readCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpReadTimeout)
	defer cancel()
	resp, err := httputil.TlsReadHttpResponse(readCtx, tlsConn)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

func webhostTcp1620check(opt WebhostSingleOpt, tlsConn *tls.UConn) error {
	defer tlsConn.Close()
	cfg := config.Get().Checkers.Webhost
	body, _ := randomBytes(cfg.Tcp1620nBytes)

	// keep-alive increases the chance that we will be able to push enough data into the connection
	req, err := http.NewRequest("POST", "https://"+opt.Host, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Close = false
	httputil.SetHeaders(&req.Header, cfg.HttpStaticHeaders)

	writeCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpWriteTimeout)
	defer cancel()
	if err := httputil.TlsWriteHttpRequest(writeCtx, tlsConn, req); err != nil {
		return err
	}

	readCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpReadTimeout)
	defer cancel()
	resp, err := httputil.TlsReadHttpResponse(readCtx, tlsConn)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
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
