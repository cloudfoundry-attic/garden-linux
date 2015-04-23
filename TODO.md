* Tidy up the `container_daemon/daemon.go`:`Handle`
  - Check error handling in JSON marshaling/Pipe creation
* Check JSON error in `container_daemon/process.go`:`NewProcess`
* Run `go vet` afterwards
* Test errors that you can test in `unix_socket/connector.go`
* Look out for pending tests
* Check wsh error message compatibility
* Rename `Execer` struct to `NamespacedExecer`
