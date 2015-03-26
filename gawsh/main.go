package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

func main() {
	switch filepath.Base(os.Args[0]) {
	case "cfsinit":
		child()
	case "wsh":
		runWsh()
	default:
		parent()
	}
}

type WshMsg struct {
	Path string
	Args []string
}

func runWsh() {
	// argument parsing
	socket := flag.String("socket", "run/wshd.sock", "socket to talk to wshd over")
	flag.String("user", "", "")
	flag.String("env", "", "")
	flag.String("pidfile", "", "")
	flag.String("dir", "", "")
	flag.Parse()

	cmd := exec.Command("sh", "-c", fmt.Sprintf("pwd; ls -l %s;", filepath.Dir(*socket)))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())

	// connect to the socket
	fmt.Printf("Connecting to socket %s\n", *socket)
	conn, err := net.Dial("unix", *socket)
	if err != nil {
		panic(fmt.Sprintf("Unable to open socket: %s", *socket))
	}

	// write arguments to socket
	fmt.Print(flag.Args())
	fmt.Print("\n")
	msg, err := json.Marshal(&WshMsg{
		Path: flag.Args()[0],
		Args: flag.Args()[1:],
	})
	must(err)
	conn.Write([]byte(msg))

	// get fds
	var b [2048]byte
	var oob [2048]byte
	n, oobn, _, _, err := conn.(*net.UnixConn).ReadMsgUnix(b[:], oob[:])
	if err != nil {
		panic(fmt.Errorf("failed to read unix msg: %s (read: %d, %d)", err, n, oobn))
	}
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		panic(fmt.Errorf("failed to parse socket control message: %s", err))
	}
	if len(scms) < 1 {
		panic(fmt.Errorf("no socket control messages sent"))
	}
	scm := scms[0]
	fds, err := syscall.ParseUnixRights(&scm)
	if err != nil {
		panic(fmt.Errorf("failed to parse unix rights: %s", err))
	}
	fmt.Println("Received fds")

	contStdin := os.NewFile(uintptr(fds[0]), "/dev/stdin")
	contStdout := os.NewFile(uintptr(fds[1]), "/dev/stdout")
	contStderr := os.NewFile(uintptr(fds[2]), "/dev/stderr")
	exitCodeFile := os.NewFile(uintptr(fds[3]), "/dev/exitcode")

	// copy std*
	go io.Copy(contStdin, os.Stdin)
	go io.Copy(os.Stdout, contStdout)
	go io.Copy(os.Stderr, contStderr)
	fmt.Println("Io.Copies are started")

	// wait for exit code
	buffer := make([]byte, 1024)
	exitCodeFile.Read(buffer)

	time.Sleep(2 * time.Second)
}

func parent() {
	// argument parsing
	rootFSPath := flag.String("root", "", "Path to root file system")
	flag.String("run", "./run", "Path to use for socket file")
	flag.String("lib", "./lib", "Hook scripts path")
	flag.String("title", "Gawsh", "Container title")
	flag.String("userns", "enabled", "Use user namespace")
	flag.Parse()

	// mount RootFS
	//must(syscall.Mount(*rootFSPath, *rootFSPath, "", uintptr(syscall.MS_BIND), ""))
	must(os.MkdirAll((*rootFSPath)+"/oldroot", 0700))

	// prepares the barrier (synchronization between child and parent) pipe
	_, containerSide, err := os.Pipe()
	if err != nil {
		panic(fmt.Sprintf("Failed to create pipe: %s", err))
	}

	// create child
	cmd := exec.Command("bash", "-c", "pwd; ls -l ./bin; chmod +x ./bin/cfsinit")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())

	big, err := filepath.Abs("./bin/cfsinit")
	must(err)

	cmd = exec.Command(big, os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: uintptr(syscall.CLONE_NEWUTS | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID)}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.ExtraFiles = append(cmd.ExtraFiles, containerSide)

	// spawn child (and new namespaces)
	fmt.Println("entering child process")
	must(cmd.Start())

	// barrier: wait for child to start
	// buffer := make([]byte, 1024)
	// _, err = hostSide.Read(buffer)
	// if err != nil {
	// 	panic(fmt.Sprintf("Unable to receive data from the container: %s", err))
	// // }
	// fmt.Printf("Received '%s'\n", buffer)
}

func child() {
	fmt.Println("in child")

	// argument parsing
	rootFSPath := flag.String("root", "", "Path to root file system")
	flag.String("run", "./run", "Path to use for socket file")
	flag.String("lib", "./lib", "Hook scripts path")
	flag.String("title", "Gawsh", "Container title")
	flag.String("userns", "enabled", "Use user namespace")
	flag.Parse()

	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX; pwd; ls -l ./run;"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())

	// socket file
	os.MkdirAll("./run", 0700)
	sock, err := net.Listen("unix", "./run/wshd.sock")
	if err != nil {
		panic("no socket :-(")
	}

	defer sock.Close()

	// pivotting here..
	must(syscall.PivotRoot(*rootFSPath, (*rootFSPath)+"/oldroot"))

	// mountpoints
	must(syscall.Mount("proc", "/proc", "proc", uintptr(0), ""))

	// host-container barrier
	containerSide := os.NewFile(uintptr(3), "/dev/hostcomm")
	_, err = containerSide.Write([]byte("after-pivot\n"))
	if err != nil {
		panic(fmt.Sprintf("Failed to send data to host: %s", err))
	}

	// environment
	must(os.Chdir("/"))

	// loop
	for {
		c, err := sock.Accept()
		if err != nil {
			panic("accept")
		}

		go func() {
			w := &WshMsg{}
			json.NewDecoder(c).Decode(w)

			// crates pipes
			var pipes [3]struct {
				r   *os.File
				w   *os.File
				err error
			}
			for i := 0; i < 3; i++ {
				pipes[i].r, pipes[i].w, pipes[i].err = os.Pipe()
			}

			fmt.Println("got wsh msg: ", w)
			// create command
			cmd := exec.Command(w.Path, w.Args...)

			// assign pipes
			cmd.Stdin = pipes[0].r
			cmd.Stdout = pipes[1].w
			cmd.Stderr = pipes[2].w

			// start the command
			must(cmd.Start())

			// make pipe for sending back exit code
			roF, wF, err := os.Pipe()
			if err != nil {
				panic(fmt.Sprintf("Cannot create pipe for processe's exit code: %s", err))
			}

			// send file descriptors
			msg := syscall.UnixRights(
				int(pipes[0].w.Fd()),
				int(pipes[1].r.Fd()),
				int(pipes[2].r.Fd()),
				int(roF.Fd()),
			)
			_, _, err = c.(*net.UnixConn).WriteMsgUnix([]byte{}, msg, nil)
			if err != nil {
				panic(fmt.Sprintf("Faled to send the fds: %s", err))
			}
			fmt.Println("Pipes are sent back to wsh")

			// write exit code in pipe
			err = cmd.Wait()
			if exitError, ok := err.(*exec.ExitError); ok {
				waitStatus := exitError.Sys().(syscall.WaitStatus)
				wF.Write([]byte(fmt.Sprintf("%d", waitStatus.ExitStatus())))
			} else {
				wF.Write([]byte("0"))
			}
		}()
	}

}

func init() {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
