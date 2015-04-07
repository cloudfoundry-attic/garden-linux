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
	"strconv"
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
	socketFilePath := flag.String("socket", "run/wshd.sock", "socket to talk to wshd over")
	flag.String("user", "", "")
	flag.String("env", "", "")
	flag.String("pidfile", "", "")
	flag.String("dir", "", "")
	flag.Parse()

	// connect to the socket
	conn, err := net.Dial("unix", *socketFilePath)
	if err != nil {
		panic(fmt.Sprintf("Unable to open socket: %s", *socketFilePath))
	}

	// write arguments to socket
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

	contStdin := os.NewFile(uintptr(fds[0]), "/dev/stdin")
	contStdout := os.NewFile(uintptr(fds[1]), "/dev/stdout")
	contStderr := os.NewFile(uintptr(fds[2]), "/dev/stderr")
	exitCodeFile := os.NewFile(uintptr(fds[3]), "/dev/exitcode")

	// copy std*
	go io.Copy(contStdin, os.Stdin)
	go io.Copy(os.Stdout, contStdout)
	go io.Copy(os.Stderr, contStderr)

	// wait for exit code
	buffer := make([]byte, 1024)
	bytes, err := exitCodeFile.Read(buffer)
	time.Sleep(2 * time.Second)
	code, err := strconv.Atoi(string(buffer[:bytes]))
	must(err)
	os.Exit(code)
}

func parent() {
	// argument parsing
	flag.String("root", "", "Path to root file system")
	flag.String("run", "./run", "Path to use for socket file")
	libDirPath := flag.String("lib", "./lib", "Hook scripts path")
	flag.String("title", "Gawsh", "Container title")
	flag.String("userns", "enabled", "Use user namespace")
	flag.Parse()

	// prepares the barrier (synchronization between child and parent) pipe
	hostSide, containerSide, err := os.Pipe()
	if err != nil {
		panic(fmt.Sprintf("Failed to create pipe: %s", err))
	}

	// run hook script
	runHookScript(*libDirPath, "parent-before-clone")

	// prepare child
	big, err := filepath.Abs("./bin/cfsinit")
	must(err)
	cmd := exec.Command(big, os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(syscall.CLONE_NEWUTS | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUSER),
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      0,
				Size:        1,
			},
		},
	}
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	// cmd.Stdin = os.Stdin
	cmd.ExtraFiles = append(cmd.ExtraFiles, containerSide)

	// spawn child (and new namespaces)
	must(cmd.Start())

	// write PID
	os.Setenv("PID", fmt.Sprintf("%d", cmd.Process.Pid))

	// barrier: wait for child to start
	buffer := make([]byte, 1024)
	_, err = hostSide.Read(buffer)
	if err != nil {
		panic(fmt.Sprintf("Unable to receive data from the container: %s", err))
	}
	fmt.Printf("Received '%s'\n", buffer)

	// run child hook script
	runHookScript(*libDirPath, "parent-after-clone")
}

func child() {
	// argument parsing
	rootFSPath := flag.String("root", "", "Path to root file system")
	runDirPath := flag.String("run", "./run", "Path to use for socket file")
	libDirPath := flag.String("lib", "./lib", "Hook scripts path")
	flag.String("title", "Gawsh", "Container title")
	flag.String("userns", "enabled", "Use user namespace")
	flag.Parse()

	// setup the barrier file
	containerSide := os.NewFile(uintptr(3), "/dev/hostcomm")

	// mount RootFS
	must(syscall.Mount(*rootFSPath, *rootFSPath, "", uintptr(syscall.MS_BIND|syscall.MS_REC), ""))

	// set proper user / group
	syscall.Setuid(0)
	syscall.Setgid(0)

	// socket file
	sock, err := net.Listen("unix", (*runDirPath)+"/wshd.sock")
	if err != nil {
		containerSide.Write([]byte(fmt.Sprintf("failed-to-open-socket: %s", err)))
		panic(fmt.Sprintf("no socket :-( %v", err))
	}
	defer sock.Close()

	// pivotting here..
	must(os.Chdir(*rootFSPath))
	must(os.Chmod("tmp", 01777))
	must(os.MkdirAll("tmp/oldroot", 0700))
	err = syscall.PivotRoot(".", "tmp/oldroot")
	if err != nil {
		containerSide.Write([]byte(fmt.Sprintf("failed-to-pivot-root: %s", err)))
		panic(fmt.Sprintf("no pivoted root :-( %v", err))
	}

	// host-container barrier
	_, err = containerSide.Write([]byte("after-pivot\n"))
	if err != nil {
		panic(fmt.Sprintf("Failed to send data to host: %s", err))
	}

	// run child hook script
	runHookScript(*libDirPath, "child-after-pivot")

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

			// create command
			cmd := exec.Command(w.Path, w.Args...)

			// assign pipes
			cmd.Stdin = pipes[0].r
			cmd.Stdout = pipes[1].w
			cmd.Stderr = pipes[2].w

			// make pipe for sending back exit code
			roF, wF, err := os.Pipe()
			if err != nil {
				panic(fmt.Sprintf("Cannot create pipe for processe's exit code: %s", err))
			}
			defer wF.Close()

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

			// start the command
			err = cmd.Start()
			if err != nil {
				pipes[2].w.Write([]byte("Program not found!\n"))
				wF.Write([]byte("255"))
				return
			}

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

func runHookScript(libDirPath string, name string) {
	cmd := exec.Command(libDirPath+"/hook", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())
}
