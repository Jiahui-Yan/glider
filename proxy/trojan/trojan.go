// protocol spec:
// https://trojan-gfw.github.io/trojan/protocol
// +-----------------------+---------+----------------+---------+----------+
// | hex(SHA224(password)) |  CRLF   | Trojan Request |  CRLF   | Payload  |
// +-----------------------+---------+----------------+---------+----------+
// |          56           | X'0D0A' |    Variable    | X'0D0A' | Variable |
// +-----------------------+---------+----------------+---------+----------+

// where Trojan Request is a SOCKS5-like request:

// +-----+------+----------+----------+
// | CMD | ATYP | DST.ADDR | DST.PORT |
// +-----+------+----------+----------+
// |  1  |  1   | Variable |    2     |
// +-----+------+----------+----------+

// where:
//     o  CMD
//         o  CONNECT X'01'
//         o  UDP ASSOCIATE X'03'
//     o  ATYP address type of following address
//         o  IP V4 address: X'01'
//         o  DOMAINNAME: X'03'
//         o  IP V6 address: X'04'
//     o  DST.ADDR desired destination address
//     o  DST.PORT desired destination port in network octet order

package trojan

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

// Trojan is a base trojan struct.
type Trojan struct {
	dialer     proxy.Dialer
	proxy      proxy.Proxy
	addr       string
	pass       [56]byte
	serverName string
	skipVerify bool
	tlsConfig  *tls.Config
}

func init() {
	proxy.RegisterDialer("trojan", NewTrojanDialer)
	// proxy.RegisterServer("trojan", NewTrojanServer)
}

// NewTrojan returns a trojan proxy.
func NewTrojan(s string, d proxy.Dialer, p proxy.Proxy) (*Trojan, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[trojan] parse err: %s", err)
		return nil, err
	}

	t := &Trojan{
		dialer: d,
		proxy:  p,
		addr:   u.Host,
	}

	// pass
	hash := sha256.New224()
	hash.Write([]byte(u.User.Username()))
	hex.Encode(t.pass[:], hash.Sum(nil))

	// serverName
	colonPos := strings.LastIndex(t.addr, ":")
	if colonPos == -1 {
		colonPos = len(t.addr)
	}
	t.serverName = t.addr[:colonPos]

	// skipVerify
	if u.Query().Get("skipVerify") == "true" {
		t.skipVerify = true
	}

	t.tlsConfig = &tls.Config{
		ServerName:         t.serverName,
		InsecureSkipVerify: t.skipVerify,
		NextProtos:         []string{"http/1.1"},
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
		MinVersion:         tls.VersionTLS10,
	}

	return t, nil
}

// NewTrojanDialer returns a trojan proxy dialer.
func NewTrojanDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewTrojan(s, d, nil)
}

// Addr returns forwarder's address.
func (s *Trojan) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *Trojan) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(rc, s.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.Write(s.pass[:])
	buf.WriteString("\r\n")
	buf.WriteByte(1)
	buf.Write(socks.ParseAddr(addr))
	buf.WriteString("\r\n")
	_, err = tlsConn.Write(buf.Bytes())
	return tlsConn, err
}

// DialUDP connects to the given address via the proxy.
func (s *Trojan) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("trojan client does not support udp now")
}