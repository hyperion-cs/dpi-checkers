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
type WebhostSingleOpt struct {
	Id           string
	Ip           netip.Addr
	Port         int
	KeyLogWriter io.Writer
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

type WebHostMode int

const (
	WebHostModePopular WebHostMode = iota
	WebHostModeInfra
)

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

type PenisOpt struct {
	Ctx  context.Context
	Mode WebHostMode
}

func WebhostGreatGochan(opt PenisOpt) <-chan WebhostSingleResult {
	cfg := config.Get().Checkers.Webhost
	sf := subnetfilter.New(inetlookup.Default())

	// итак, нам надо:
	// паралельно запустить subnetfilter для каждого name
	// дождаться
	// параллельно запустить webhostfarmer для каждого name
	// не дожидаясь, параллельно запустить webhost (check) для тех ферм, что уже готовы..
	// и все это пихать в общий чан с говном

	// при этом, у каждого из этим модулей свои воркеры встроенные ;)

	if opt.Mode == WebHostModePopular {
		panic("not impl yet")
	}

	// opt.Mode == WebHostModeInfra

	sfGochanIn := make(chan subnetfilter.RunFilterGochanIn)
	sfGochan := sf.RunFilterGochan(subnetfilter.RunFilterGochanOpt{Ctx: opt.Ctx, In: sfGochanIn})
	// нагрузим работой subnetfilter
	gochan.Push(opt.Ctx, sfGochanIn, getSubnetfilterItems(sf, opt.Mode))

	farmGochanIn := make(chan webhostfarm.FarmGochanIn)
	farmGochan := webhostfarm.FarmGochan(webhostfarm.FarmGochanOpt{Ctx: opt.Ctx, In: farmGochanIn})

	// нагрузим работой фарму, результатами из subnetfilter
	go func() {
		defer close(farmGochanIn)
		for x := range sfGochan {
			fmt.Println("id:", x.Id, "subnetfilter prefixes:", len(x.IpSet.Prefixes()))
			// TODO: откуда тут брать count?
			in := webhostfarm.FarmGochanIn{Id: x.Id, Opt: webhostfarm.FarmOpt{Subnets: x.IpSet, Count: 3}}
			select {
			case <-opt.Ctx.Done():
				return
			case farmGochanIn <- in:
			}
		}
	}()

	// наконец, нагрузим работой чекер, результатами из фармы
	var keyLogWriter io.Writer
	var postFunc func()
	if cfg.KeyLogPath != "" {
		file, err := os.OpenFile(cfg.KeyLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			panic(err)
		}
		postFunc = func() { file.Close() }
		keyLogWriter = file
	}
	webhostGochanIn := make(chan WebhostSingleOpt)
	webhostGochan := WebhostGochan(WebhostGochanOpt{Ctx: opt.Ctx, In: webhostGochanIn, Post: postFunc})

	go func() {
		defer close(webhostGochanIn)
		for x := range farmGochan {
			fmt.Println("id:", x.Id, "farm items:", len(x.FarmItems))
			for _, v := range x.FarmItems {
				in := WebhostSingleOpt{Id: x.Id, Ip: v.Ip, Port: v.Port, KeyLogWriter: keyLogWriter}
				select {
				case <-opt.Ctx.Done():
					return
				case webhostGochanIn <- in:
				}
			}
		}

	}()

	return webhostGochan
}

type WebhostGochanOpt struct {
	Ctx  context.Context
	In   <-chan WebhostSingleOpt
	Post func()
}

func WebhostGochan(opt WebhostGochanOpt) <-chan WebhostSingleResult {
	cfg := config.Get().Checkers.Webhost
	return gochan.Start(gochan.GochanOpt[WebhostSingleOpt, WebhostSingleResult]{
		Ctx:      opt.Ctx,
		Workers:  cfg.Workers,
		Input:    opt.In,
		Executor: Webhost,
		Post:     opt.Post,
	})
}

func getSubnetfilterItems(sf *subnetfilter.Subnetfilter, mode WebHostMode) []subnetfilter.RunFilterGochanIn {
	cfg := config.Get().Checkers.Webhost
	iter := cfg.Infra

	if mode == WebHostModePopular {
		iter = cfg.Popular
	}

	items := []subnetfilter.RunFilterGochanIn{}
	for _, v := range iter {
		// TODO: handle errors
		f, _ := sf.CompileFilter(v.Filter)
		items = append(items, subnetfilter.RunFilterGochanIn{Id: v.Name, Filter: f})
	}
	return items
}

func Webhost(opt WebhostSingleOpt) WebhostSingleResult {
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
	if err = webhostAliveCheck(tlsConn); err != nil {
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
	if err = webhostTcp1620check(tlsConn); err != nil {
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

	// without sni
	tlsConn := tls.UClient(tcpConn, &tls.Config{
		InsecureSkipVerify: true,
		KeyLogWriter:       keyLogWriter,
	}, tls.HelloCustom)

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
	defer tlsConn.Close()
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
	defer tlsConn.Close()
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
