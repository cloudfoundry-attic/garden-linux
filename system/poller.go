package system

type pollfd struct {
	fd      int32
	events  int16
	revents int16
}

type Poller struct {
	fds []pollfd
}
