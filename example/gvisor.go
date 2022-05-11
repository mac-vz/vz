package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/dustin/go-humanize"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var (
	debug     bool
	mtu       int
	unixgram  net.Conn
	endpoints arrayFlags
)

const (
	gatewayIP   = "192.168.127.1"
	sshHostPort = "192.168.127.2:22"
)

func StartProxy(listenDebug bool, macAddr string, vzDataGram net.Conn) {
	unixgram = vzDataGram
	debug = listenDebug
	mtu = 1500
	ctx, cancel := context.WithCancel(context.Background())

	groupErrs, ctx := errgroup.WithContext(ctx)

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	//Set this as we want Protocol stream to be off
	protocol := types.BessProtocol

	config := types.Configuration{
		Debug:             debug,
		CaptureFile:       captureFile(),
		MTU:               mtu,
		Subnet:            "192.168.127.0/24",
		GatewayIP:         gatewayIP,
		GatewayMacAddress: "5a:94:ef:e4:0c:dd",
		DHCPStaticLeases: map[string]string{
			"192.168.127.2": macAddr,
		},
		Forwards: map[string]string{
			fmt.Sprintf("127.0.0.1:%d", 2222): sshHostPort,
		},
		DNS: []types.Zone{
			{
				Name: "docker.internal.",
				Records: []types.Record{
					{
						Name: "gateway",
						IP:   net.ParseIP(gatewayIP),
					},
					{
						Name: "host",
						IP:   net.ParseIP("192.168.127.254"),
					},
				},
			},
		},
		DNSSearchDomains: searchDomains(),
		NAT: map[string]string{
			"192.168.127.254": "127.0.0.1",
		},
		GatewayVirtualIPs: []string{"192.168.127.254"},
		Protocol:          protocol,
	}

	groupErrs.Go(func() error {
		err := run(ctx, groupErrs, &config, []string{"unix:///Users/balaji/Desktop/GitSource/Otto/vz/example/vm.sock"})
		fmt.Printf("Context done", err)
		return err
	})

	// Wait for something to happen
	groupErrs.Go(func() error {
		select {
		// Catch signals so exits are graceful and defers can run
		//case <-errCh:
		//	logrus.Infof("Stopping all")
		//	cancel()
		//	return errors.New("signal caught")
		case <-ctx.Done():
			logrus.Infof("Context done")
			cancel()
			return nil
		}
	})
	//if err := groupErrs.Wait(); err != nil {
	//	logrus.Error(err)
	//	exitCode = 1
	//}
}

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func captureFile() string {
	if !debug {
		return ""
	}
	return "capture.pcap"
}

func run(ctx context.Context, g *errgroup.Group, configuration *types.Configuration, endpoints []string) error {
	vn, err := virtualnetwork.New(configuration)
	if err != nil {
		return err
	}
	logrus.Info("waiting for clients...")

	for _, endpoint := range endpoints {
		logrus.Infof("listening %s", endpoint)
		ln, err := transport.Listen(endpoint)
		if err != nil {
			return errors.Wrap(err, "cannot listen")
		}
		httpServe(ctx, g, ln, withProfiler(vn))
	}

	ln, err := vn.Listen("tcp", fmt.Sprintf("%s:80", gatewayIP))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/services/forwarder/all", vn.Mux())
	mux.Handle("/services/forwarder/expose", vn.Mux())
	mux.Handle("/services/forwarder/unexpose", vn.Mux())
	httpServe(ctx, g, ln, mux)

	if debug {
		g.Go(func() error {
		debugLog:
			for {
				select {
				case <-time.After(5 * time.Second):
					fmt.Printf("%v sent to the VM, %v received from the VM\n", humanize.Bytes(vn.BytesSent()), humanize.Bytes(vn.BytesReceived()))
				case <-ctx.Done():
					break debugLog
				}
			}
			return nil
		})
	}

	if unixgram != nil {

		g.Go(func() error {
			<-ctx.Done()
			if err := unixgram.Close(); err != nil {
				log.Println("error closing %s: %q", unixgram, err)
			}
			return err
		})

		g.Go(func() error {
			fmt.Printf("Starting connection")
			return vn.AcceptQemu(ctx, unixgram)
		})
	}

	return nil
}

func httpServe(ctx context.Context, g *errgroup.Group, ln net.Listener, mux http.Handler) {
	g.Go(func() error {
		<-ctx.Done()
		return ln.Close()
	})
	g.Go(func() error {
		err := http.Serve(ln, mux)
		if err != nil {
			if err != http.ErrServerClosed {
				return err
			}
			return err
		}
		return nil
	})
}

func withProfiler(vn *virtualnetwork.VirtualNetwork) http.Handler {
	mux := vn.Mux()
	if debug {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	}
	return mux
}

func exitWithError(err error) {
	logrus.Infof("Data error", err)
	logrus.Error(err)
	os.Exit(1)
}

func searchDomains() []string {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		f, err := os.Open("/etc/resolv.conf")
		if err != nil {
			logrus.Errorf("open file error: %v", err)
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		searchPrefix := "search "
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), searchPrefix) {
				searchDomains := strings.Split(strings.TrimPrefix(sc.Text(), searchPrefix), " ")
				logrus.Debugf("Using search domains: %v", searchDomains)
				return searchDomains
			}
		}
		if err := sc.Err(); err != nil {
			logrus.Errorf("scan file error: %v", err)
			return nil
		}
	}
	return nil
}
