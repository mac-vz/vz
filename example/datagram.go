package main

import (
	"net"
	"os"
	"reflect"
	"time"
)

func GetFdFromConn(l net.Conn) int {
	v := reflect.ValueOf(l)
	netFD := reflect.Indirect(reflect.Indirect(v).FieldByName("fd"))
	netFD = netFD.FieldByName("pfd")
	fd := int(netFD.FieldByName("Sysfd").Int())
	return fd
}

func ListenUnixGram(path string) *net.UnixConn {
	if _, err := os.Stat(path); err == nil {
		err := os.Remove(path)
		if err != nil {
			log.Fatal("Error during delete of unixgram", path, err)
		}
	}
	unixgram, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		log.Fatal("Error listening to unixgram", path, err)
	}
	return unixgram
}

func DialUnixGram(clientPath string, serverPath string) *net.UnixConn {
	if _, err := os.Stat(clientPath); err == nil {
		err := os.Remove(clientPath)
		if err != nil {
			log.Fatal("Error during delete of client unixgram", clientPath, err)
		}
	}
	unixgram, err := net.DialUnix("unixgram", &net.UnixAddr{Name: clientPath, Net: "unixgram"},
		&net.UnixAddr{Name: serverPath, Net: "unixgram"})
	if err != nil {
		log.Fatal("Error listening to unixgram", serverPath, err)
	}
	return unixgram
}

type FDConnection struct {
	FD    int
	File  *os.File
	LAddr net.UnixAddr // local
	RAddr net.UnixAddr // remote
}

var _ net.Conn = (*FDConnection)(nil)

// Read reads data from connection of the vsock protocol.
func (v *FDConnection) Read(b []byte) (n int, err error) {
	return v.File.Read(b)
}

// Write writes data to the connection of the vsock protocol.
func (v *FDConnection) Write(b []byte) (n int, err error) {
	return v.File.Write(b)
}

// Close will be called when caused something error in socket.
func (v *FDConnection) Close() error {
	return v.File.Close()
}

// LocalAddr returns the local network address.
func (v *FDConnection) LocalAddr() net.Addr {
	return &v.LAddr
}

// RemoteAddr returns the remote network address.
func (v *FDConnection) RemoteAddr() net.Addr {
	return &v.RAddr
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
func (v *FDConnection) SetDeadline(t time.Time) error {
	return v.File.SetDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (v *FDConnection) SetReadDeadline(t time.Time) error {
	return v.File.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (v *FDConnection) SetWriteDeadline(t time.Time) error {
	return v.File.SetWriteDeadline(t)
}
