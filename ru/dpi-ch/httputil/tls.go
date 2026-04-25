package httputil

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	tls "github.com/refraction-networking/utls"
)

var (
	ErrTcpConnReset          = errors.New("tcp: connection reset")
	ErrTcpConnTimeout        = errors.New("tcp: connection timeout")
	ErrTcpWriteTimeout       = errors.New("tcp: write timeout")
	ErrTcpReadTimeout        = errors.New("tcp: read timeout")
	ErrTlsCertificateInvalid = errors.New("tls: certificate invalid")
	ErrTlsHandshakeTimeout   = errors.New("tls: handshake timeout")
	ErrTlsHandshakeFail      = errors.New("tls: handshake failure")
	ErrTlsBadRecordMac       = errors.New("tls: bad record MAC")
	ErrTlsWriteBrokenPipe    = errors.New("tls: broken write pipe")
	ErrInternal              = errors.New("net: internal error")
)

type TlsConnOpt struct {
	Ctx                 context.Context
	Ip                  netip.Addr
	Port                int
	Sni                 string
	TcpConnTimeout      time.Duration
	TcpWriteBuf         int
	TcpReadBuf          int
	TlsHandshakeTimeout time.Duration
	KeyLogWriter        io.Writer
	InsecureVerify      bool
}

// TODO (options):
// - Set proto (http/https)
// - Set tlsV
// - Try to extract sni/host from cert
func GetHandshakedUTlsConn(opt TlsConnOpt) (*tls.UConn, error) {
	tcpDialer := net.Dialer{}
	if opt.TcpConnTimeout != 0 {
		tcpDialer.Timeout = opt.TcpConnTimeout
	}

	addr := net.JoinHostPort(opt.Ip.String(), strconv.Itoa(opt.Port))
	tcpConn, err := tcpDialer.Dial("tcp", addr)
	if err != nil {
		if isTimeoutErr(err) {
			return nil, ErrTcpConnTimeout
		}
		if handledErr, ok := tryHandleErr(err); ok {
			return nil, handledErr
		}

		log.Println("getHandshakedUTlsConn/Dial", err)
		return nil, ErrInternal
	}

	rawTcpConn := tcpConn.(*net.TCPConn)
	if opt.TcpWriteBuf != 0 {
		rawTcpConn.SetWriteBuffer(opt.TcpWriteBuf)
	}
	if opt.TcpReadBuf != 0 {
		rawTcpConn.SetReadBuffer(opt.TcpReadBuf)
	}

	tlsConf := &tls.Config{
		InsecureSkipVerify: !opt.InsecureVerify,
		KeyLogWriter:       opt.KeyLogWriter,
	}

	if opt.Sni != "" {
		tlsConf.ServerName = opt.Sni
	}

	tlsConn := tls.UClient(tcpConn, tlsConf, tls.HelloCustom)

	// chrome fingerprint originally contains ALPN for h2
	chromeSpec, _ := tls.UTLSIdToSpec(tls.HelloChrome_133)
	setUTlsAlpn(&chromeSpec, []string{"http/1.1"})
	tlsConn.ApplyPreset(&chromeSpec)

	if opt.TlsHandshakeTimeout != 0 {
		tlsConn.SetDeadline(time.Now().Add(opt.TlsHandshakeTimeout))
		defer tlsConn.SetDeadline(time.Time{})
	}

	if opt.Ctx == nil {
		err = tlsConn.Handshake()
	} else {
		err = tlsConn.HandshakeContext(opt.Ctx)
	}

	if err != nil {
		if isTimeoutErr(err) {
			return nil, ErrTlsHandshakeTimeout
		}
		if handledErr, ok := tryHandleErr(err); ok {
			return nil, handledErr
		}

		log.Println("getHandshakedUTlsConn/Handshake", err)
		return nil, ErrInternal
	}

	return tlsConn, nil
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

func TlsReadHttpResponse(ctx context.Context, tlsConn *tls.UConn, br *bufio.Reader) (*http.Response, error) {
	done := make(chan struct{})
	defer close(done)
	defer tlsConn.SetReadDeadline(time.Time{})

	go func() {
		select {
		case <-ctx.Done():
			_ = tlsConn.SetReadDeadline(time.Now())
		case <-done:
		}
	}()

	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		if isTimeoutErr(err) {
			return nil, ErrTcpReadTimeout
		}
		if handledErr, ok := tryHandleErr(err); ok {
			return nil, handledErr
		}
		log.Println("TlsReadHttpResponse", err)
		return nil, ErrInternal
	}
	return resp, nil
}

func TlsWriteHttpRequest(ctx context.Context, tlsConn *tls.UConn, req *http.Request) (int64, error) {
	done := make(chan struct{})
	defer close(done)
	defer tlsConn.SetWriteDeadline(time.Time{})

	go func() {
		select {
		case <-ctx.Done():
			_ = tlsConn.SetWriteDeadline(time.Now())
		case <-done:
		}
	}()

	var writeBuf bytes.Buffer
	if err := req.Write(&writeBuf); err != nil {
		return 0, err
	}
	n := int64(writeBuf.Len())

	if _, err := tlsConn.Write(writeBuf.Bytes()); err != nil {
		if isTimeoutErr(err) {
			return 0, ErrTcpWriteTimeout
		}
		if handledErr, ok := tryHandleErr(err); ok {
			return 0, handledErr
		}
		log.Println("TlsHttpRequest", err)
		return 0, ErrInternal
	}
	return n, nil
}

func IsHttputilErr(err error) bool {
	switch err {
	case ErrTcpConnReset, ErrTcpConnTimeout, ErrTcpWriteTimeout,
		ErrTcpReadTimeout, ErrTlsCertificateInvalid, ErrTlsHandshakeTimeout,
		ErrTlsHandshakeFail, ErrTlsBadRecordMac, ErrTlsWriteBrokenPipe,
		ErrInternal:
		return true
	default:
		return false
	}
}

func isTimeoutErr(err error) bool {
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

// Try to handle errors. Assume that timeouts are already handled.
func tryHandleErr(err error) (error, bool) {
	// yeah, it looks like shit. but there's nothing we can do about it ;(

	// alert errors: https://go.dev/src/crypto/tls/alert.go
	if strings.Contains(err.Error(), "handshake failure") {
		return ErrTlsHandshakeFail, true
	}
	if strings.Contains(err.Error(), "bad record MAC") {
		return ErrTlsBadRecordMac, true
	}

	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return ErrTlsCertificateInvalid, true
	}

	// others
	if strings.Contains(err.Error(), "connection reset") {
		return ErrTcpConnReset, true
	}
	if strings.Contains(err.Error(), "write: broken pipe") {
		return ErrTlsWriteBrokenPipe, true
	}

	return err, false
}
