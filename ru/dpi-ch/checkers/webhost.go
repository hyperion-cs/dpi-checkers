package checkers

import (
	"context"
	"crypto/rand"
	"dpich/config"
	"dpich/gochan"
	"dpich/inetlookup"
	"dpich/subnetfilter"
	"dpich/webhostfarm"
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
// - Set sni/host
// - Set random sni/host (?)
// - Try to extract sni/host from cert
// - Skip tcp1620 check (alive only)
type WebhostOpt struct {
	Ip           netip.Addr
	Port         int
	KeyLogWriter io.Writer
}

type WebhostAttr struct {
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
	ErrWebhostTcpConnReset        = errors.New("tcp connection reset")
	ErrWebhostTcpConnTimeout      = errors.New("tcp connection timeout")
	ErrWebhostTlsHandshakeTimeout = errors.New("tls handshake timeout")
	ErrWebhostTlsHandshakeFail    = errors.New("tls handshake failure")
	ErrWebhostTcpWriteTimeout     = errors.New("tcp write timeout")
	ErrWebhostTcpReadTimeout      = errors.New("tcp read timeout")
	ErrWebhostInternal            = errors.New("internal error")
	ErrWebhostSkip                = errors.New("skip")
)

func WebhostStart(ctx context.Context) <-chan WebhostAttr {
	cfg := config.Get().Checkers.Webhost
	sf := subnetfilter.New(inetlookup.Default())
	//f, _ := sf.CompileFilter(`org("hetzner")`)

	f, _ := sf.CompileFilter(`subnet("195.201.92.197/32")`)
	subnets, _ := sf.RunFilter(f)
	fmt.Println("found subnets:", len(subnets.Prefixes()))
	items := webhostfarm.Farm(webhostfarm.FarmOpt{Subnets: subnets, Count: 1})
	fmt.Printf("ips: %v\n", items)

	var keyLogWriter io.Writer
	if cfg.KeyLogPath != "" {
		file, err := os.OpenFile(cfg.KeyLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		keyLogWriter = file
	}

	in := make(chan WebhostOpt)
	out := gochan.Start(gochan.GochanOpt[WebhostOpt, WebhostAttr]{
		Ctx:      ctx,
		Workers:  cfg.CheckWorkers,
		Input:    in,
		Executor: WebhostSingle,
	})

	go func() {
		defer close(in)
		for _, x := range items {
			select {
			case <-ctx.Done():
				return
			case in <- WebhostOpt{Ip: x.Ip, Port: x.Port, KeyLogWriter: keyLogWriter}:
			}
		}
	}()

	return out
}

func WebhostSingle(opt WebhostOpt) WebhostAttr {
	if opt.KeyLogWriter == nil {
		opt.KeyLogWriter = io.Discard
	}

	ipinfo := inetlookup.Default().IpInfo(opt.Ip)
	a := WebhostAttr{IpInfo: ipinfo, Port: opt.Port}

	// alive check
	tlsConn, err := getHandshakedUTlsConn(opt, opt.KeyLogWriter)
	if err != nil {
		a.Alive = err
		a.Tcp1620 = ErrWebhostSkip
		return a
	}
	defer tlsConn.Close()
	a.TlsV = tlsConn.ConnectionState().Version

	if err = webhostAliveCheck(tlsConn); err != nil {
		a.Alive = err
		a.Tcp1620 = ErrWebhostSkip
		return a
	}

	// tcp16-20 check
	tlsConn, err = getHandshakedUTlsConn(opt, opt.KeyLogWriter)
	if err != nil {
		a.Tcp1620 = err
		return a
	}
	defer tlsConn.Close()

	if err = webhostTcp1620check(tlsConn); err != nil {
		a.Tcp1620 = err
	}

	return a
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

func getHandshakedUTlsConn(opt WebhostOpt, keyLogWriter io.Writer) (*tls.UConn, error) {
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

	// without sni
	tlsConn := tls.UClient(tcpConn, &tls.Config{
		InsecureSkipVerify: true,
		KeyLogWriter:       keyLogWriter,
	}, tls.HelloCustom)

	// chrome fingerprint originally contains ALPN for h2
	chromeSpec, _ := tls.UTLSIdToSpec(tls.HelloChrome_Auto)
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
		log.Println("getHandshakedUTlsConn/Handshake", err)
		return nil, ErrWebhostInternal
	}
	tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}

func tlsReadAll(tlsConn *tls.UConn) ([]byte, error) {
	cfg := config.Get().Checkers.Webhost
	tlsConn.SetReadDeadline(time.Now().Add(cfg.TcpReadTimeout))
	defer tlsConn.SetReadDeadline(time.Time{})
	data, err := io.ReadAll(tlsConn)
	if err != nil {
		if isTimeout(err) {
			return nil, ErrWebhostTcpReadTimeout
		}
		if strings.Contains(err.Error(), "connection reset") {
			return nil, ErrWebhostTcpConnReset
		}
		log.Println("tlsReadAll", err)
		return nil, ErrWebhostInternal
	}
	return data, nil
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

func webhostAliveCheck(tlsConn *tls.UConn) error {
	req := prepareHttpReq(webhostHttpReq{method: "HEAD"})
	if err := tlsWriteAll(tlsConn, req); err != nil {
		return err
	}
	if _, err := tlsReadAll(tlsConn); err != nil {
		return err
	}
	return nil
}

func webhostTcp1620check(tlsConn *tls.UConn) error {
	cfg := config.Get().Checkers.Webhost
	body, _ := randomBytes(cfg.Tcp1620nBytes)
	req := prepareHttpReq(webhostHttpReq{method: "POST", body: body})
	if err := tlsWriteAll(tlsConn, req); err != nil {
		return err
	}
	if _, err := tlsReadAll(tlsConn); err != nil {
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
		reqStr += "Host: " + opt.host
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
