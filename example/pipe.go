package main

import (
	"net"
	"time"
)

type UDPConn struct {
	unixConn *net.UnixConn
	writeTo  net.Addr
}

func (udp *UDPConn) Read(b []byte) (n int, err error) {
	log.Println("Test Read")
	from, addr, err := udp.unixConn.ReadFrom(b)
	udp.writeTo = addr
	log.Println("Test Read", addr)
	return from, err
}

func (udp *UDPConn) Write(b []byte) (n int, err error) {
	log.Println("Test Write")
	return udp.unixConn.WriteTo(b, udp.writeTo)
}

func (udp *UDPConn) Close() error {
	//return udp.unixConn.Close()
	return nil
}

func (udp *UDPConn) LocalAddr() net.Addr {
	return udp.unixConn.LocalAddr()
}

func (udp *UDPConn) RemoteAddr() net.Addr {
	if udp.writeTo == nil {
		return &net.UnixAddr{Name: "", Net: ""}
	}
	return udp.writeTo
}

func (udp *UDPConn) SetDeadline(t time.Time) error {
	return udp.unixConn.SetDeadline(t)
}

func (udp *UDPConn) SetReadDeadline(t time.Time) error {
	return udp.unixConn.SetReadDeadline(t)
}

func (udp *UDPConn) SetWriteDeadline(t time.Time) error {
	return udp.unixConn.SetWriteDeadline(t)
}
