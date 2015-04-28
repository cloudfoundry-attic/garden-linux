* Tidy up the `container_daemon/daemon.go`:`Handle`
  - Check error handling in JSON marshaling/Pipe creation
* Check JSON error in `container_daemon/process.go`:`NewProcess`
* Run `go vet` afterwards (pick up untested/caught errors)
* Test errors that you can test in `unix_socket/connector.go`
* Look out for pending tests
* Check wsh error message compatibility
* do mount_linux_tests actually need runInContainer?
* handle panics and send proper errors
