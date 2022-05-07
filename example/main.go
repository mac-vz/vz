package main

import (
	"github.com/Code-Hex/vz"
	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
	l "log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var log *l.Logger

// https://developer.apple.com/documentation/virtualization/running_linux_in_a_virtual_machine?language=objc#:~:text=Configure%20the%20Serial%20Port%20Device%20for%20Standard%20In%20and%20Out
func setRawMode(f *os.File) {
	var attr unix.Termios

	// Get settings for terminal
	termios.Tcgetattr(f.Fd(), &attr)

	// Put stdin into raw mode, disabling local echo, input canonicalization,
	// and CR-NL mapping.
	attr.Iflag &^= syscall.ICRNL
	attr.Lflag &^= syscall.ICANON | syscall.ECHO

	// Set minimum characters when reading = 1 char
	attr.Cc[syscall.VMIN] = 1

	// set timeout when reading as non-canonical mode
	attr.Cc[syscall.VTIME] = 0

	// reflects the changed settings
	termios.Tcsetattr(f.Fd(), termios.TCSANOW, &attr)
}

func main() {
	StartProxy("/Users/balaji/Desktop/GitSource/Otto/vz/example/network.sock", false, "/Users/balaji/Desktop/GitSource/Otto/vz/example/main.sock")
	file, err := os.Create("./log.log")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	log = l.New(file, "", l.LstdFlags)

	kernelCommandLineArguments := []string{
		// Use the first virtio console device as system console.
		"console=hvc0",
		// Stop in the initial ramdisk before attempting to transition to
		// the root file system.
		"root=/dev/vda",
	}

	vmlinuz := "/Users/balaji/Desktop/ubuntu-focal-20.04/original/vmlinux"
	initrd := "/Users/balaji/Desktop/ubuntu-focal-20.04/original/initrd"
	diskPath := "/Users/balaji/Desktop/ubuntu-focal-20.04/original/ubuntu-20.04-km-disk.img"

	bootLoader := vz.NewLinuxBootLoader(
		vmlinuz,
		vz.WithCommandLine(strings.Join(kernelCommandLineArguments, " ")),
		vz.WithInitrd(initrd),
	)
	log.Println("bootLoader:", bootLoader)

	config := vz.NewVirtualMachineConfiguration(
		bootLoader,
		1,
		2*1024*1024*1024,
	)

	setRawMode(os.Stdin)

	// console
	serialPortAttachment := vz.NewFileHandleSerialPortAttachment(os.Stdin, os.Stdout)
	consoleConfig := vz.NewVirtioConsoleDeviceSerialPortConfiguration(serialPortAttachment)
	config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{
		consoleConfig,
	})

	// network
	//hostSock := "/Users/balaji/Desktop/GitSource/Otto/vz/example/server.sock"
	//gram := ListenUnixGram(hostSock)
	//go func() {
	//	StartProxy(false, "5a:94:ef:e4:0c:ee", gram)
	//}()
	//
	//clientNet := DialUnixGram("/Users/balaji/Desktop/GitSource/Otto/vz/example/client.sock", hostSock)
	//
	//fd := GetFdFromConn(clientNet)
	//
	//natAttachment := vz.NewFileHandleNetworkDeviceAttachment(fd)
	//networkConfig := vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
	//mac, err := net.ParseMAC("5a:94:ef:e4:0c:ee")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//networkConfig.SetMACAddress(vz.NewMACAddress(mac))

	//config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{
	//	networkConfig,
	//})

	// entropy
	entropyConfig := vz.NewVirtioEntropyDeviceConfiguration()
	config.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{
		entropyConfig,
	})

	diskImageAttachment, err := vz.NewDiskImageStorageDeviceAttachment(
		diskPath,
		false,
	)
	if err != nil {
		log.Fatal(err)
	}
	storageDeviceConfig := vz.NewVirtioBlockDeviceConfiguration(diskImageAttachment)
	config.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{
		storageDeviceConfig,
	})

	configuration := vz.NewVZVirtioFileSystemDeviceConfiguration("test", "/Users/balaji", false)
	config.SetDirectorySharingDevices([]vz.DirectorySharingDeviceConfiguration{
		configuration,
	})

	// traditional memory balloon device which allows for managing guest memory. (optional)
	config.SetMemoryBalloonDevicesVirtualMachineConfiguration([]vz.MemoryBalloonDeviceConfiguration{
		vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration(),
	})

	// socket device (optional)
	config.SetSocketDevicesVirtualMachineConfiguration([]vz.SocketDeviceConfiguration{
		vz.NewVirtioSocketDeviceConfiguration(),
	})
	validated, err := config.Validate()
	if !validated || err != nil {
		log.Fatal("validation failed", err)
	}

	vm := vz.NewVirtualMachine(config)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM)

	errCh := make(chan error, 1)

	vm.Start(func(err error) {
		if err != nil {
			errCh <- err
		}
	})

	for {
		select {
		case <-signalCh:
			result, err := vm.RequestStop()
			if err != nil {
				log.Println("request stop error:", err)
				return
			}
			log.Println("recieved signal", result)
		case newState := <-vm.StateChangedNotify():
			if newState == vz.VirtualMachineStateRunning {
				log.Println("start VM is running")
				time.Sleep(30 * time.Second)
				err = ExposeVsock(vm, 1024, "/Users/balaji/Desktop/GitSource/Otto/vz/example/main.sock")
			}
			if newState == vz.VirtualMachineStateStopped {
				log.Println("stopped successfully")
				return
			}
		case err := <-errCh:
			log.Println("in start:", err)
		}
	}

	// vm.Resume(func(err error) {
	// 	fmt.Println("in resume:", err)
	// })
}
