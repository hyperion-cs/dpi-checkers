package webhost

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

const (
	randomHostnameAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomHostnameLen      = 12
)

type SingleOpt struct {
	Ctx            context.Context
	Ip             netip.Addr
	Port           int
	Sni            string
	Host           string
	Tcp1620skip    bool
	SiberianSkip   bool
	RandomHostname bool
}

type Tls struct {
	V   uint16
	San []string
	Cn  string
}

type SingleResult struct {
	IpInfo   inetlookup.IpInfo
	Port     int
	Tls      *Tls
	Sni      string
	Host     string
	Alive    error
	Tcp1620  error
	Siberian error

	// Set only if Tcp1620 == nil
	Throughput Throughput
}

type Throughput struct {
	TxBytes   int64
	RxBytes   int64
	TxElapsed time.Duration
	RxElapsed time.Duration
}

var (
	ErrBlockedBySni = errors.New("tls: blocked by sni") // Displayed verbatim in the TUI.
	ErrInternal     = errors.New("webhost: internal error")
	ErrSkip         = errors.New("webhost: skip")
)

func (t *Tls) VersionString() string {
	if t == nil {
		return " — "
	}

	switch t.V {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return " — "
	}
}

// Kbps returns both rates when both directions were measured.
func (t Throughput) Kbps() (*float64, *float64) {
	if t.TxElapsed <= 0 || t.RxElapsed <= 0 {
		return nil, nil
	}

	const bytesToKilobits = 8.0 / 1_000
	txKbps := float64(t.TxBytes) / t.TxElapsed.Seconds() * bytesToKilobits
	rxKbps := float64(t.RxBytes) / t.RxElapsed.Seconds() * bytesToKilobits
	return &txKbps, &rxKbps
}

func Single(opt SingleOpt) SingleResult {
	if opt.RandomHostname {
		rndHostname, _ := randomHostname()
		opt.Sni = rndHostname
		opt.Host = rndHostname
	}

	ipinfo := inetlookup.Default().IpInfo(opt.Ip)
	res := SingleResult{
		IpInfo: ipinfo,
		Port:   opt.Port,
		Sni:    opt.Sni,
		Host:   opt.Host,
	}

	cfg := config.Get().Checkers.Webhost
	tlsConnOpt := inetutil.TlsConnOpt{
		Ctx:                 opt.Ctx,
		Ip:                  opt.Ip,
		Port:                opt.Port,
		Sni:                 opt.Sni,
		TcpConnTimeout:      cfg.TcpConnTimeout,
		TcpWriteBuf:         cfg.TcpWriteBuf,
		TcpReadBuf:          cfg.TcpReadBuf,
		TlsHandshakeTimeout: cfg.TlsHandshakeTimeout,
	}

	tlsConn, err := checkHandshakes(opt, tlsConnOpt)
	if err != nil {
		res.Alive = err
		res.Tcp1620 = ErrSkip
		res.Siberian = ErrSkip
		return res
	}
	defer tlsConn.Close()

	tlsConnState := tlsConn.ConnectionState()
	res.Tls = &Tls{V: tlsConnState.Version}
	if len(tlsConnState.PeerCertificates) > 0 {
		cert := tlsConnState.PeerCertificates[0]
		log.Println("webhost; ip: ", opt.Ip, "san: ", cert.DNSNames, "cn: ", cert.Subject.CommonName)
		res.Tls.San = cert.DNSNames
		res.Tls.Cn = cert.Subject.CommonName
	}

	if opt.Ctx.Err() != nil {
		res.Tcp1620 = ErrSkip
		res.Siberian = ErrSkip
		return res
	}

	// The order of the checks is important.

	res.Alive = checkAlive(opt, tlsConn)
	if res.Alive != nil && res.Alive != inetutil.ErrHttpMalformedResponse {
		res.Tcp1620 = ErrSkip
		res.Siberian = ErrSkip
		return res
	}

	if opt.Tcp1620skip || opt.Ctx.Err() != nil {
		res.Tcp1620 = ErrSkip
	} else {
		thp, err := checkTcp1620(opt, tlsConnOpt)
		if err != nil {
			res.Tcp1620 = err
		}
		res.Throughput = thp
	}

	if opt.SiberianSkip || opt.Ctx.Err() != nil {
		res.Siberian = ErrSkip
	} else {
		res.Siberian = checkSiberian(tlsConnOpt)
	}

	return res
}

func checkHandshakes(opt SingleOpt, tlsConnOpt inetutil.TlsConnOpt) (*tls.UConn, error) {
	tlsConn, err := inetutil.GetHandshakedUTlsConn(tlsConnOpt)
	if inetutil.IsInetutilErr(err) && !opt.RandomHostname && tlsConnOpt.Sni != "" {
		// TODO: We can determine this in 1 pass using Org by IP + move that into config.
		snis := []string{"cf.com", "google.com"}
		for _, s := range snis {
			tlsConnOpt.Sni = s
			tlsConn, err = inetutil.GetHandshakedUTlsConn(tlsConnOpt)
			if err == nil {
				tlsConn.Close()
				err = ErrBlockedBySni
				break
			}
		}

	}
	return tlsConn, err
}

func checkAlive(opt SingleOpt, tlsConn *tls.UConn) error {
	cfg := config.Get().Checkers.Webhost

	req, err := http.NewRequest("HEAD", "https://"+opt.Host, http.NoBody)
	if err != nil {
		return err
	}
	req.Close = true
	inetutil.SetHeaders(&req.Header, cfg.HttpStaticHeaders)

	writeCtx, cancel := context.WithTimeout(opt.Ctx, cfg.TcpWriteTimeout)
	defer cancel()
	if _, err := inetutil.TlsWriteHttpRequest(writeCtx, tlsConn, req); err != nil {
		return err
	}

	readCtx, cancel := context.WithTimeout(opt.Ctx, cfg.TcpReadTimeout)
	defer cancel()
	stopClose := context.AfterFunc(readCtx, func() { tlsConn.Close() })
	defer stopClose()
	resp, err := inetutil.TlsReadHttpResponse(readCtx, tlsConn, bufio.NewReader(tlsConn))
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

func checkTcp1620(opt SingleOpt, tlsConnOpt inetutil.TlsConnOpt) (Throughput, error) {
	tlsConn, err := inetutil.GetHandshakedUTlsConn(tlsConnOpt)
	if err != nil {
		return Throughput{}, err
	}
	defer tlsConn.Close()

	cfg := config.Get().Checkers.Webhost
	body, _ := randomBytes(cfg.Tcp1620nBytes)

	// keep-alive increases the chance that we will be able to push enough data into the connection
	req, err := http.NewRequest("POST", "https://"+opt.Host, bytes.NewReader(body))
	if err != nil {
		return Throughput{}, err
	}
	req.Close = false
	inetutil.SetHeaders(&req.Header, cfg.HttpStaticHeaders)

	writeCtx, cancel := context.WithTimeout(opt.Ctx, cfg.TcpWriteTimeout)
	defer cancel()
	txStart := time.Now()
	txBytes, err := inetutil.TlsWriteHttpRequest(writeCtx, tlsConn, req)
	if err != nil {
		return Throughput{}, err
	}
	txElapsed := time.Since(txStart)

	if opt.Ctx.Err() != nil {
		return Throughput{}, ErrSkip
	}

	readCtx, cancel := context.WithTimeout(opt.Ctx, cfg.TcpReadTimeout)
	defer cancel()
	stopClose := context.AfterFunc(readCtx, func() { tlsConn.Close() })
	defer stopClose()
	rxCr := &inetutil.CountingReader{Reader: tlsConn}
	rxStart := time.Now()
	resp, err := inetutil.TlsReadHttpResponse(readCtx, tlsConn, bufio.NewReader(rxCr))

	if err != nil && err != inetutil.ErrHttpMalformedResponse {
		return Throughput{}, err
	}

	if err == nil {
		if _, err = io.Copy(io.Discard, resp.Body); err != nil {
			if errors.Is(readCtx.Err(), context.DeadlineExceeded) {
				return Throughput{}, inetutil.ErrTcpReadTimeout
			}
			return Throughput{}, err
		}
		defer resp.Body.Close()
	}

	rxElapsed := time.Since(rxStart)
	return Throughput{
		TxBytes:   txBytes,
		TxElapsed: txElapsed,
		RxBytes:   rxCr.Bytes,
		RxElapsed: rxElapsed,
	}, nil
}

func checkSiberian(tlsConnOpt inetutil.TlsConnOpt) error {
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

	log.Println("webhost; webhostSiberianCheck ip:", tlsConnOpt.Ip, "port:",
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
	b := make([]byte, randomHostnameLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	const rha = randomHostnameAlphabet
	for i := range b {
		b[i] = rha[int(b[i])%len(rha)]
	}

	return fmt.Sprintf("%s.%s", string(b), tld), nil
}
