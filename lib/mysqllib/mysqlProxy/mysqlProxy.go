package mysqlProxy

import (
	"context"
	"errors"
	"fmt"
	"golangApp/lib/clsr"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/net/proxy"
)

type Proxy struct {
	ctx           context.Context
	Config        *ssh.ClientConfig
	client        *ssh.Client
	localListener net.Listener
	LocalConn     net.Conn
	RemoteConn    net.Conn

	*sync.RWMutex

	auth  *proxy.Auth
	recvb int64
	sendb int64

	localAddress  *atomic.Value
	remoteAddress *atomic.Value

	remotePort uint32
	localPort  uint32

	errs chan error
}

func (p *Proxy) LocalPort() uint16 {
	return uint16(atomic.LoadUint32(&p.localPort))
}

func (p *Proxy) RemotePort() uint16 {
	return uint16(atomic.LoadUint32(&p.remotePort))
}

func (p *Proxy) Errors() <-chan error {
	return p.errs
}

func (p *Proxy) Start() (err error) {
	p.client, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", p.LocalHost(), 22), p.Config)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to dial SSH server: %s ", err.Error()))
	}

	fmt.Println(fmt.Sprintf("%s:%d", p.LocalHost(), 22), fmt.Sprintf("127.0.0.1:%d", p.LocalPort()))
	p.localListener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.LocalPort()))
	if err != nil {
		return errors.New(fmt.Sprintf("failed to listen on local port %d: %v", p.LocalPort(), err))
	}
	return nil
}

func (p *Proxy) Close() error {
	var err error
	if p.RemoteConn != nil {
		err := p.RemoteConn.Close()
		if err != nil {
			fmt.Printf("failed to close remote connection: %v", err)
		}
	}

	if p.LocalConn != nil {
		err = p.LocalConn.Close()
		if err != nil {
			fmt.Printf("failed to close local connection: %v", err)
		}
	}

	if p.client != nil {
		err = p.client.Close()
		if err != nil {
			fmt.Printf("failed to close client: %v", err)
		}
	}

	if p.localListener != nil {
		err = p.localListener.Close()
		if err != nil {
			fmt.Printf("failed to close local listener: %v", err)
		}
	}
	close(p.errs)
	return nil
}

func (p *Proxy) RemoteHost() string {
	x, _ := p.remoteAddress.Load().(string)
	return x
}

func (p *Proxy) LocalHost() string {
	x, _ := p.localAddress.Load().(string)
	return x
}

func (p *Proxy) StartSend() {
	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			default:
				var err error
				p.Lock()
				p.LocalConn, err = p.localListener.Accept()
				p.Unlock()
				if err != nil {
					p.errs <- err
					continue
				}

				p.Lock()
				p.RemoteConn, err = p.client.Dial("tcp", fmt.Sprintf("%s:%d", p.RemoteHost(), p.RemotePort()))
				p.Unlock()
				if err != nil {
					p.errs <- err
					p.LocalConn.Close()
					continue
				}

				p.RemoteToLocal()
				p.LocalToRemote()
			}
		}
	}()
}

func (p *Proxy) RemoteToLocal() {
	go func() {
		select {
		case <-p.ctx.Done():
			return
		default:
			p.RLock()
			size, err := io.Copy(p.RemoteConn, p.LocalConn)
			if err != nil {
				p.errs <- err
				return
			}
			atomic.AddInt64(&p.recvb, size)
			p.RemoteConn.Close()
			p.RUnlock()
		}
	}()
}

func (p *Proxy) LocalToRemote() {
	go func() {
		select {
		case <-p.ctx.Done():
			return
		default:
			p.RLock()
			size, err := io.Copy(p.LocalConn, p.RemoteConn)
			if err != nil {
				p.errs <- err
				return
			}
			atomic.AddInt64(&p.sendb, size)
			p.LocalConn.Close()
			p.RUnlock()
		}
	}()
}

type EnvAuth struct{}

func (s EnvAuth) Credentials() *proxy.Auth {
	return &proxy.Auth{
		User:     os.Getenv("USERNAME"),
		Password: os.Getenv("PASSWORD"),
	}
}

type ProxyArgs struct {
	Config                      *ssh.ClientConfig
	HostKeyCallback             ssh.HostKeyCallback
	LocalAddress, RemoteAddress string
	LocalPort, RemotePort       uint16
	*proxy.Auth
}

func NewProxy(ctx context.Context, cc *clsr.CleanupComplete, args *ProxyArgs) (*Proxy, error) {
	la := &atomic.Value{}
	la.Store(args.LocalAddress)

	ra := &atomic.Value{}
	ra.Store(args.RemoteAddress)

	var err error
	if args.HostKeyCallback == nil {
		args.HostKeyCallback, err = knownhosts.New("C:\\Users\\jdk_d\\.ssh\\known_hosts") // Optional: use proper host key verification
		if err != nil {
			log.Println("Warning: Using InsecureIgnoreHostKey. This is insecure.")
			args.HostKeyCallback = ssh.InsecureIgnoreHostKey()
			return nil, err
		}
	}

	if args.Config == nil {
		args.Config = &ssh.ClientConfig{
			User: args.User,
			Auth: []ssh.AuthMethod{
				ssh.Password(args.Password),
			},
			HostKeyCallback: args.HostKeyCallback,
			Timeout:         30 * time.Second,
		}
	}

	prx := &Proxy{
		ctx:           ctx,
		Config:        args.Config,
		auth:          args.Auth,
		recvb:         0,
		sendb:         0,
		localAddress:  la,
		RWMutex:       &sync.RWMutex{},
		remoteAddress: ra,
		remotePort:    uint32(args.RemotePort),
		localPort:     uint32(args.LocalPort),
		errs:          make(chan error, 1),
	}
	clsr.SigClosure(nil, cc, prx)
	return prx, nil
}
