package receiver

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"sync"

	"github.com/lomik/go-carbon/helper"
	"github.com/lomik/go-carbon/points"
	"github.com/lomik/zapwriter"
)

type Receiver interface {
	Stop()
	Stat(helper.StatCallback)
}

var protocolMap = map[string](func(name string, dsn string, store func(*points.Points)) (Receiver, error)){}
var protocolMapMutex sync.Mutex

func Register(protocol string, constructor func(name string, dsn string, store func(*points.Points)) (Receiver, error)) {
	protocolMapMutex.Lock()
	defer protocolMapMutex.Unlock()

	_, ok := protocolMap[protocol]
	if ok {
		log.Fatalf("protocol %#v already registered", protocol)
	}

	protocolMap[protocol] = constructor
}

func New(name string, dsn string, store func(*points.Points)) (Receiver, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}

	protocolMapMutex.Lock()
	constructor, ok := protocolMap[u.Scheme]
	protocolMapMutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("unknown protocol %#v", u.Scheme)
	}

	return constructor(name, dsn, store)
}

type Option func(Receiver) error

// PickleMaxMessageSize creates option for New contructor
func PickleMaxMessageSize(size uint32) Option {
	return func(r Receiver) error {
		if t, ok := r.(*TCP); ok {
			t.maxPickleMessageSize = size
		}
		return nil
	}
}

// UDPLogIncomplete creates option for New contructor
func UDPLogIncomplete(enable bool) Option {
	return func(r Receiver) error {
		if t, ok := r.(*UDP); ok {
			t.logIncomplete = enable
		}
		return nil
	}
}

// OutChan creates option for New contructor
func OutChan(ch chan *points.Points) Option {
	return OutFunc(func(p *points.Points) {
		ch <- p
	})
}

// OutFunc creates option for New contructor
func OutFunc(out func(*points.Points)) Option {
	return func(r Receiver) error {
		if t, ok := r.(*TCP); ok {
			t.out = out
		}
		if t, ok := r.(*UDP); ok {
			t.out = out
		}
		return nil
	}
}

// BufferSize creates option for New contructor
func BufferSize(size int) Option {
	return func(r Receiver) error {
		if t, ok := r.(*TCP); ok {
			if size == 0 {
				t.buffer = nil
			} else {
				t.buffer = make(chan *points.Points, size)
			}
		}
		if t, ok := r.(*UDP); ok {
			if size == 0 {
				t.buffer = nil
			} else {
				t.buffer = make(chan *points.Points, size)
			}
		}
		return nil
	}
}

// Name creates option for New contructor
func Name(name string) Option {
	return func(r Receiver) error {
		if t, ok := r.(*TCP); ok {
			t.name = name
		}
		if t, ok := r.(*UDP); ok {
			t.name = name
		}
		return nil
	}
}

func blackhole(p *points.Points) {}

// New creates udp, tcp, pickle receiver
func _New(dsn string, opts ...Option) (Receiver, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}

	if u.Scheme == "tcp" || u.Scheme == "pickle" {
		addr, err := net.ResolveTCPAddr("tcp", u.Host)
		if err != nil {
			return nil, err
		}

		r := &TCP{
			out:    blackhole,
			name:   u.Scheme,
			logger: zapwriter.Logger(u.Scheme),
		}

		if u.Scheme == "pickle" {
			r.isPickle = true
			r.maxPickleMessageSize = 67108864 // 64Mb
		}

		for _, optApply := range opts {
			optApply(r)
		}

		if err = r.Listen(addr); err != nil {
			return nil, err
		}

		return r, err
	}

	if u.Scheme == "udp" {
		addr, err := net.ResolveUDPAddr("udp", u.Host)
		if err != nil {
			return nil, err
		}

		r := &UDP{
			out:    blackhole,
			name:   u.Scheme,
			logger: zapwriter.Logger(u.Scheme),
		}

		for _, optApply := range opts {
			optApply(r)
		}

		err = r.Listen(addr)
		if err != nil {
			return nil, err
		}

		return r, err
	}

	return nil, fmt.Errorf("unknown proto %#v", u.Scheme)
}
