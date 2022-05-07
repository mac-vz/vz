package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/gorilla/handlers"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

var (
	debug bool
)

const (
	gatewayIP   = "192.168.127.1"
	sshHostPort = "192.168.127.2:22"
)

func vsockListener(tap string) (net.Listener, error) {
	_ = os.Remove(tap)
	ln, err := net.Listen("unix", tap)
	logrus.Infof("listening %s", tap)
	if err != nil {
		return nil, err
	}
	return ln, nil
}

func httpListener(network string) (net.Listener, error) {
	_ = os.Remove(network)
	ln, err := net.Listen("unix", network)
	logrus.Infof("listening %s", network)
	if err != nil {
		return nil, err
	}
	return ln, nil
}

func StartProxy(network string, listenDebug bool, qemuSocket string) {
	debug = listenDebug
	ctx, cancel := context.WithCancel(context.Background())

	groupErrs, ctx := errgroup.WithContext(ctx)

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	config := types.Configuration{
		Debug:             debug,
		CaptureFile:       captureFile(),
		MTU:               4000,
		Subnet:            "192.168.127.0/24",
		GatewayIP:         gatewayIP,
		GatewayMacAddress: "5a:94:ef:e4:0c:dd",
		DHCPStaticLeases: map[string]string{
			"192.168.127.2": "5a:94:ef:e4:0c:ee",
		},
		Forwards: map[string]string{
			fmt.Sprintf("127.0.0.1:%d", 2223): sshHostPort,
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
		GatewayVirtualIPs: []string{"192.168.127.254"},
		Protocol:          types.HyperKitProtocol,
	}

	groupErrs.Go(func() error {
		return run(ctx, groupErrs, &config, network, qemuSocket)
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

func captureFile() string {
	if !debug {
		return ""
	}
	return "capture.pcap"
}

func run(ctx context.Context, g *errgroup.Group, configuration *types.Configuration, endpoint string, qemu string) error {
	vsockListener, err := vsockListener(qemu)
	if err != nil {
		return err
	}

	errCh := make(chan error)

	vn, err := virtualnetwork.New(configuration)
	if err != nil {
		return err
	}
	logrus.Info("waiting for clients...")

	httpListener, err := httpListener(endpoint)
	if err != nil {
		return errors.Wrap(err, "cannot listen")
	}
	go func() {
		if httpListener == nil {
			return
		}
		mux := http.NewServeMux()
		mux.Handle("/network/", http.StripPrefix("/network", vn.Mux()))
		if err := http.Serve(httpListener, handlers.LoggingHandler(os.Stderr, mux)); err != nil {
			errCh <- errors.Wrap(err, "api http.Serve failed")
		}
	}()

	ln, err := vn.Listen("tcp", fmt.Sprintf("%s:80", configuration.GatewayIP))
	if err != nil {
		return err
	}
	go func() {
		mux := gatewayAPIMux()
		if err := http.Serve(ln, handlers.LoggingHandler(os.Stderr, mux)); err != nil {
			errCh <- errors.Wrap(err, "gateway http.Serve failed")
		}
	}()

	networkListener, err := vn.Listen("tcp", fmt.Sprintf("%s:7777", "192.168.127.254"))
	if err != nil {
		return err
	}
	go func() {
		mux := networkAPIMux(vn)
		if err := http.Serve(networkListener, handlers.LoggingHandler(os.Stderr, mux)); err != nil {
			errCh <- errors.Wrap(err, "host virtual IP http.Serve failed")
		}
	}()

	go func() {
		mux := http.NewServeMux()
		mux.Handle(types.ConnectPath, vn.Mux())
		if err := http.Serve(vsockListener, mux); err != nil {
			errCh <- errors.Wrap(err, "virtualnetwork http.Serve failed")
		}
	}()

	c := make(chan os.Signal, 1)

	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	select {
	case <-c:
		return nil
	case err := <-errCh:
		return err
	}

	return nil
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

// This API is only exposed in the virtual network (only the VM can reach this).
// Any process inside the VM can reach it by connecting to gateway.crc.testing:80.
func gatewayAPIMux() *http.ServeMux {
	mux := http.NewServeMux()
	//mux.HandleFunc("/hosts/add", func(w http.ResponseWriter, r *http.Request) {
	//	acceptJSONStringArray(w, r, func(hostnames []string) error {
	//		return adminhelper.AddToHostsFile("127.0.0.1", hostnames...)
	//	})
	//})
	//mux.HandleFunc("/hosts/remove", func(w http.ResponseWriter, r *http.Request) {
	//	acceptJSONStringArray(w, r, func(hostnames []string) error {
	//		return adminhelper.RemoveFromHostsFile(hostnames...)
	//	})
	//})
	return mux
}

func networkAPIMux(vn *virtualnetwork.VirtualNetwork) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", vn.Mux())
	return mux
}

func acceptJSONStringArray(w http.ResponseWriter, r *http.Request, fun func(hostnames []string) error) {
	if r.Method != http.MethodPost {
		http.Error(w, "post only", http.StatusBadRequest)
		return
	}
	var req []string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := fun(req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
