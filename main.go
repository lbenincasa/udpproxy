package main

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

var opts struct {
	Source string   `long:"source" default:":2203" description:"Source port to listen on"`
	Target []string `long:"target" description:"Target address to forward to"`
	Quiet  bool     `long:"quiet" description:"whether to print logging info or not"`
	Server bool     `long:"server" description:"server or client mode (only with nats)"`
	Nats   bool     `long:"nats" description:"nats mode"`
	Buffer int      `long:"buffer" default:"10240" description:"max buffer size for the socket io"`
}

func main() {
	_, err := flags.Parse(&opts)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			log.Printf("error: %v\n", err.Error())
			os.Exit(1)
		} else {
			// log.Printf("%v\n", err.Error())
			os.Exit(0)
		}
	}

	if opts.Quiet {
		log.SetLevel(log.WarnLevel)
	}

	sourceAddr, err := net.ResolveUDPAddr("udp", opts.Source)
	if err != nil {
		log.WithError(err).Fatal("Could not resolve source address:", opts.Source)
		return
	}

	var targetAddr []*net.UDPAddr
	for _, v := range opts.Target {
		addr, err := net.ResolveUDPAddr("udp", v)
		if err != nil {
			log.WithError(err).Fatal("Could not resolve target address:", v)
			return
		}
		targetAddr = append(targetAddr, addr)
	}

	sourceConn, err := net.ListenUDP("udp", sourceAddr)
	if err != nil {
		log.WithError(err).Fatal("Could not listen on address:", opts.Source)
		return
	}

	defer sourceConn.Close()

	var sub *nats.Subscription
	var nc *nats.Conn
	if opts.Nats {
		// Connect to a server
		nc, err = nats.Connect(nats.DefaultURL)
		if err != nil {
			log.WithError(err).Fatal("Could not connect to nats server: ", nats.DefaultURL)
			return
		}
		log.Printf(">> Connection with nats server success (%v)", nats.DefaultURL)
		defer nc.Close()

		if opts.Server {
			// Nats: Simple Sync Subscriber
			sub, err = nc.SubscribeSync("udpproxy")
			if err != nil {
				log.WithError(err).Fatal("Could not subscribe to foo")
				return
			}
		}
	}

	var targetConn []*net.UDPConn
	for _, v := range targetAddr {
		conn, err := net.DialUDP("udp", nil, v)
		if err != nil {
			log.WithError(err).Fatal("Could not connect to target address:", v)
			return
		}

		defer conn.Close()
		targetConn = append(targetConn, conn)
	}

	log.Printf(">> Starting udpproxy, Source at %v, Target at %v...", opts.Source, opts.Target)

	b := make([]byte, opts.Buffer)
	for {
		if opts.Nats {
			if opts.Server {
				//	m, err := sub.NextMsg(nats.DefaultTimeout)
				log.Printf(">> wait a msg from nats server...")
				msg, err := sub.NextMsg(100 * time.Hour)
				//			if err != nil || !bytes.Equal(msg.Data, []byte("omsg")) {
				if err != nil {
					log.WithError(err).Fatal("Could not receive msg")
					continue
				}
				log.WithField("content", string(msg.Data)).Info("Packet received")

				for _, v := range targetConn {
					if _, err := v.Write(msg.Data[0:]); err != nil {
						log.WithError(err).Warn("Could not forward packet.")
					}
				}
			} else {
				n, addr, err := sourceConn.ReadFromUDP(b)
				if err != nil {
					log.WithError(err).Error("Could not receive a packet")
					continue
				}
				log.WithField("addr", addr.String()).WithField("bytes", n).Info("Packet received")

				err = nc.Publish("udpproxy", b[0:n])
				if err != nil {
					log.WithError(err).Error("Could not publish msg to nats!!")
					continue
				}
			}

		} else {
			n, addr, err := sourceConn.ReadFromUDP(b)
			if err != nil {
				log.WithError(err).Error("Could not receive a packet")
				continue
			}
			log.WithField("addr", addr.String()).WithField("bytes", n).WithField("content", string(b)).Info("Packet received")

			for _, v := range targetConn {
				if _, err := v.Write(b[0:n]); err != nil {
					log.WithError(err).Warn("Could not forward packet.")
				}
			}
		}
	}
}
