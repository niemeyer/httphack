package httphack

import (
	"net"
	"net/url"
	"strings"
	"sync"
)

const debug = false

func debugln(args ...interface{}) {
	if debug {
		debugln(args...)
	}
}

type tunnel struct {
	lstn  net.Listener
	lconn net.Conn
	rconn net.Conn
	raddr string
	mu    sync.Mutex
	done  chan error
	lockw chan bool
	lockr chan bool
	unlockw chan bool
	unlockr chan bool
}

func host(rawurl string) string {
	url, err := url.Parse(rawurl)
	if err != nil {
		panic("invalid url: " + rawurl)
	}
	return url.Host
}

func newTunnel(raddr string) *tunnel {
	lstn, err := net.Listen("tcp4", "")
	if err != nil {
		panic(err)
	}
	debugln("listening on", lstn.Addr().String())
	t := &tunnel{
		lstn:    lstn,
		raddr:   raddr,
		lockw:   make(chan bool, 1),
		lockr:   make(chan bool, 1),
		unlockw: make(chan bool, 1),
		unlockr: make(chan bool, 1),
		done:    make(chan error, 1),
	}
	go t.loop()
	return t
}

func (t *tunnel) Addr() string {
	s := t.lstn.Addr().String()
	i := strings.Index(s, ":")
	return "127.0.0.1" + s[i:]
}

func (t *tunnel) Close() error {
	var err error
	select {
	case err = <-t.done:
	default:
	}
	debugln("closing listener")
	t.lstn.Close()
	t.mu.Lock()
	debugln("closing all connections")
	if t.lconn != nil {
		t.lconn.Close()
	}
	if t.rconn != nil {
		t.rconn.Close()
	}
	t.mu.Unlock()
	if err == nil {
		<-t.done
	}
	return err
}

func (t *tunnel) LockWrite() {
	t.lockw <- true
}

func (t *tunnel) LockRead() {
	t.lockr <- true
}

func (t *tunnel) UnlockWrite() {
	select {
	case <-t.lockw:
	default:
		t.unlockw <- true
	}
}

func (t *tunnel) UnlockRead() {
	select {
	case <-t.lockr:
	default:
		t.unlockr <- true
	}
}

func (t *tunnel) loop() {
	buf := make([]byte, 4096)
	for {
		var err error
		t.mu.Lock()
		debugln("waiting for local connection")
		t.lconn, err = t.lstn.Accept()
		if err == nil {
			debugln("accepted local, dialing remote:", t.raddr)
			t.rconn, err = net.Dial("tcp", t.raddr)
		}
		lconn := t.lconn
		rconn := t.rconn
		t.mu.Unlock()
		if err != nil {
			debugln("tunnel stopping")
			t.done <- err
			return
		}
		debugln("tunnel is ready")

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for {
				n, err := lconn.Read(buf)
				if err != nil {
					debugln("read error from lconn:", err.Error())
					return
				}
				debugln("read", n, "bytes from local")
				select {
				case <-t.lockw:
					debugln("write is locked")
					<-t.unlockw
					debugln("write is unlocked")
				default:
				}
				n, err = rconn.Write(buf[:n])
				if err != nil {
					debugln("write error from rconn:", err.Error())
					return
				}
				debugln("wrote", n, "bytes to remote")
			}
		}()
		go func() {
			defer wg.Done()
			for {
				n, err := rconn.Read(buf)
				if err != nil {
					debugln("read error from rconn:", err.Error())
					return
				}
				debugln("read", n, "bytes from remote")
				select {
				case <-t.lockr:
					debugln("read is locked")
					<-t.unlockr
					debugln("read is unlocked")
				default:
				}
				n, err = lconn.Write(buf[:n])
				if err != nil {
					debugln("write error from lconn:", err.Error())
					return
				}
				debugln("wrote", n, "bytes to local")
			}
		}()
		wg.Wait()
		rconn.Close()
		lconn.Close()
		debugln("tunnel is closed")
	}
}
