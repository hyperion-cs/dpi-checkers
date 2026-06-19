package checkers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"time"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	"golang.org/x/sync/errgroup"

	tls "github.com/refraction-networking/utls"
)

type WebhostSingleOpt struct {
	Ip             netip.Addr
	Port           int
	Sni            string
	Host           string
	Tcp1620skip    bool
	SiberianSkip   bool
	RandomHostname bool
}

type WebhostSingleResult struct {
	IpInfo   inetlookup.IpInfo
	Port     int
	TlsV     uint16
	Sni      string
	Host     string
	Alive    error
	Tcp1620  error
	Siberian error

	// Set only if Tcp1620 == nil
	Throughput WebhostThroughput
}

type WebhostThroughput struct {
	TxBytes   int64
	RxBytes   int64
	TxElapsed time.Duration
	RxElapsed time.Duration
}

var (
	ErrWebhostInternal = errors.New("check: internal error")
	ErrWebhostSkip     = errors.New("check: skip")
)

const RANDOM_HOSTNAME_ALPHABET = "abcdefghijklmnopqrstuvwxyz0123456789"
const RANDOM_HOSTNAME_LEN = 12

func WebhostSingle(opt WebhostSingleOpt) WebhostSingleResult {
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
	tlsConnOpt := inetutil.TlsConnOpt{
		Ip:                  opt.Ip,
		Port:                opt.Port,
		Sni:                 opt.Sni,
		TcpConnTimeout:      cfg.TcpConnTimeout,
		TcpWriteBuf:         cfg.TcpWriteBuf,
		TcpReadBuf:          cfg.TcpReadBuf,
		TlsHandshakeTimeout: cfg.TlsHandshakeTimeout,
	}

	// TODO: Move conn setup to webhostAliveCheck
	tlsConn, err := inetutil.GetHandshakedUTlsConn(tlsConnOpt)
	if err != nil {
		res.Alive = err
		res.Tcp1620 = ErrWebhostSkip
		res.Siberian = ErrWebhostSkip
		return res
	}
	res.TlsV = tlsConn.ConnectionState().Version

	// The order of the checks is important.

	if err = webhostAliveCheck(opt, tlsConn); err != nil {
		res.Alive = err
		res.Tcp1620 = ErrWebhostSkip
		res.Siberian = ErrWebhostSkip
		return res
	}

	if opt.Tcp1620skip {
		res.Tcp1620 = ErrWebhostSkip
	} else {
		thp, err := webhostTcp1620check(opt, tlsConnOpt)
		if err != nil {
			res.Tcp1620 = err
		}
		res.Throughput = thp
	}

	if opt.SiberianSkip {
		res.Siberian = ErrWebhostSkip
	} else {
		res.Siberian = webhostSiberianCheck(tlsConnOpt)
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
	inetutil.SetHeaders(&req.Header, cfg.HttpStaticHeaders)

	writeCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpWriteTimeout)
	defer cancel()
	if _, err := inetutil.TlsWriteHttpRequest(writeCtx, tlsConn, req); err != nil {
		return err
	}

	readCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpReadTimeout)
	defer cancel()
	resp, err := inetutil.TlsReadHttpResponse(readCtx, tlsConn, bufio.NewReader(tlsConn))
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

func webhostTcp1620check(opt WebhostSingleOpt, tlsConnOpt inetutil.TlsConnOpt) (WebhostThroughput, error) {
	tlsConn, err := inetutil.GetHandshakedUTlsConn(tlsConnOpt)
	if err != nil {
		return WebhostThroughput{}, err
	}
	defer tlsConn.Close()

	cfg := config.Get().Checkers.Webhost
	body, _ := randomBytes(cfg.Tcp1620nBytes)

	// keep-alive increases the chance that we will be able to push enough data into the connection
	req, err := http.NewRequest("POST", "https://"+opt.Host, bytes.NewReader(body))
	if err != nil {
		return WebhostThroughput{}, err
	}
	req.Close = false
	inetutil.SetHeaders(&req.Header, cfg.HttpStaticHeaders)

	writeCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpWriteTimeout)
	defer cancel()
	txStart := time.Now()
	txBytes, err := inetutil.TlsWriteHttpRequest(writeCtx, tlsConn, req)
	if err != nil {
		return WebhostThroughput{}, err
	}
	txElapsed := time.Since(txStart)

	readCtx, cancel := context.WithTimeout(context.Background(), cfg.TcpReadTimeout)
	defer cancel()
	rxCr := &inetutil.CountingReader{Reader: tlsConn}
	rxStart := time.Now()
	resp, err := inetutil.TlsReadHttpResponse(readCtx, tlsConn, bufio.NewReader(rxCr))
	if err != nil {
		return WebhostThroughput{}, err
	}
	if _, err = io.Copy(io.Discard, resp.Body); err != nil {
		return WebhostThroughput{}, err
	}
	rxElapsed := time.Since(rxStart)
	resp.Body.Close()

	return WebhostThroughput{
		TxBytes:   txBytes,
		TxElapsed: txElapsed,
		RxBytes:   rxCr.Bytes,
		RxElapsed: rxElapsed,
	}, nil
}

func webhostSiberianCheck(tlsConnOpt inetutil.TlsConnOpt) error {
	cfg := config.Get().Checkers.Webhost
	fingerprint := inetutil.Fingerprints[cfg.SiberianFingerprint]
	if fingerprint == nil {
		panic(fmt.Sprintf(`inetutil=>siberian-fingerprint / invalid value: "%s".`, cfg.SiberianFingerprint))
	}
	tlsConnOpt.ClientHelloId = *fingerprint
	tlsConnOpt.OriginalAlpn = true

	origSni := tlsConnOpt.Sni       // "valid" only if there is info in config
	alphaSni, _ := randomHostname() // random sni allows to reset restriction context
	betaSni, _ := randomHostname()

	tlsConnOpt.Sni = alphaSni
	alpha := siberianCheckSeq(tlsConnOpt, cfg.SiberianConnCount)
	tlsConnOpt.Sni = betaSni
	beta := siberianCheckSeq(tlsConnOpt, 1)

	// Some services may return a TLS error if the SNI is random.
	// In this case, it makes sense to try with a valid SNI first, and then with an empty one.
	if alpha == inetutil.ErrTlsInternal || beta == inetutil.ErrTlsInternal {
		alphaSni = origSni
		betaSni = "" // as far as we know, empty SNI does not trigger "siberian" restrictions

		tlsConnOpt.Sni = alphaSni
		alpha = siberianCheckSeq(tlsConnOpt, cfg.SiberianConnCount)
		tlsConnOpt.Sni = betaSni
		beta = siberianCheckSeq(tlsConnOpt, 1)
	}

	log.Println("webhost: webhostSiberianCheck ip:", tlsConnOpt.Ip, "port:",
		tlsConnOpt.Port, "alpha sni:", alphaSni, "result:", alpha, "beta sni:", betaSni, "res:", beta)

	if (alpha != nil && beta == nil) ||
		(alpha == inetutil.ErrTlsHandshakeTimeout && beta == inetutil.ErrTlsHandshakeTimeout) {
		return alpha
	}
	return nil
}

func siberianCheckSeq(tlsConnOpt inetutil.TlsConnOpt, count int) error {
	// TODO: remove tlsConnOpt control from here
	if tlsConnOpt.Ctx == nil {
		tlsConnOpt.Ctx = context.Background()
	}
	g, ctx := errgroup.WithContext(tlsConnOpt.Ctx)
	tlsConnOpt.Ctx = ctx

	for range count {
		g.Go(func() error {
			tlsConn, err := inetutil.GetHandshakedUTlsConn(tlsConnOpt)
			if err != nil {
				return err
			}
			defer tlsConn.Close()
			return nil
		})
	}
	return g.Wait()
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
