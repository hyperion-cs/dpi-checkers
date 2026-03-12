package httputil

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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

var (
	ErrTcpConnReset        = errors.New("tcp: connection reset")
	ErrTcpConnTimeout      = errors.New("tcp: connection timeout")
	ErrTcpWriteTimeout     = errors.New("tcp: write timeout")
	ErrTcpReadTimeout      = errors.New("tcp: read timeout")
	ErrTlsHandshakeTimeout = errors.New("tls: handshake timeout")
	ErrTlsHandshakeFail    = errors.New("tls: handshake failure")
	ErrTlsBadRecordMac     = errors.New("tls: bad record MAC")
	ErrTlsWriteBrokenPipe  = errors.New("tls: broken write pipe")
	ErrInternal            = errors.New("net: internal error")
)

type TlsConnOpt struct {
	Ip                  netip.Addr
	Port                int
	Sni                 string
	TcpConnTimeout      time.Duration
	TcpWriteBuf         int
	TcpReadBuf          int
	TlsHandshakeTimeout time.Duration
	KeyLogWriter        io.Writer
}

// TODO (options):
// - Set proto (http/https)
// - Set tlsV
// - Try to extract sni/host from cert
func GetHandshakedUTlsConn(opt TlsConnOpt) (*tls.UConn, error) {
	tcpDialer := net.Dialer{Timeout: opt.TcpConnTimeout}
	addr := net.JoinHostPort(opt.Ip.String(), strconv.Itoa(opt.Port))

	tcpConn, err := tcpDialer.Dial("tcp", addr)
	if err != nil {
		if isTimeoutErr(err) {
			return nil, ErrTcpConnTimeout
		}
		if whErr, ok := tryHandleErr(err); ok {
			return nil, whErr
		}

		log.Println("getHandshakedUTlsConn/Dial", err)
		return nil, ErrInternal
	}

	rawTcpConn := tcpConn.(*net.TCPConn)
	rawTcpConn.SetWriteBuffer(opt.TcpWriteBuf)
	rawTcpConn.SetReadBuffer(opt.TcpReadBuf)

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
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

	tlsConn.SetDeadline(time.Now().Add(opt.TlsHandshakeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		if isTimeoutErr(err) {
			return nil, ErrTlsHandshakeTimeout
		}
		if whErr, ok := tryHandleErr(err); ok {
			return nil, whErr
		}

		log.Println("getHandshakedUTlsConn/Handshake", err)
		return nil, ErrInternal
	}
	tlsConn.SetDeadline(time.Time{})
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

func TlsReadHttpHeaders(tlsConn *tls.UConn, timeout time.Duration) ([]byte, error) {
	tlsConn.SetReadDeadline(time.Now().Add(timeout))
	defer tlsConn.SetReadDeadline(time.Time{})

	br := bufio.NewReader(tlsConn)
	var buf []byte
	needle := []byte("\r\n\r\n")

	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			// TODO: No error?
			if err == io.EOF {
				return buf, nil
			}
			if isTimeoutErr(err) {
				return nil, ErrTcpReadTimeout
			}
			if whErr, ok := tryHandleErr(err); ok {
				return nil, whErr
			}
			log.Println("tlsReadHttpHeaders", err)
			return nil, ErrInternal
		}

		buf = append(buf, line...)
		if bytes.HasSuffix(buf, needle) {
			return buf, nil
		}
	}
}

func TlsWriteAll(tlsConn *tls.UConn, data []byte, timeout time.Duration) error {
	tlsConn.SetWriteDeadline(time.Now().Add(timeout))
	defer tlsConn.SetWriteDeadline(time.Time{})
	if _, err := tlsConn.Write(data); err != nil {
		if isTimeoutErr(err) {
			return ErrTcpWriteTimeout
		}
		if whErr, ok := tryHandleErr(err); ok {
			return whErr
		}
		log.Println("tlsWriteAll", err)
		return ErrInternal
	}
	return nil
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

	// others
	if strings.Contains(err.Error(), "connection reset") {
		return ErrTcpConnReset, true
	}
	if strings.Contains(err.Error(), "write: broken pipe") {
		return ErrTlsWriteBrokenPipe, true
	}

	return err, false
}
