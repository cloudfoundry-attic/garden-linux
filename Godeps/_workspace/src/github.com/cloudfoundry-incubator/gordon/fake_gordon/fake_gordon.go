package fake_gordon

import (
	"io/ioutil"
	"os"
	"sync"

	"code.google.com/p/gogoprotobuf/proto"
	"github.com/cloudfoundry-incubator/gordon"
	"github.com/cloudfoundry-incubator/gordon/warden"
	"github.com/nu7hatch/gouuid"
)

type FakeGordon struct {
	Connected    bool
	ConnectError error

	createdHandles    []string
	createdProperties map[string]map[string]string
	CreateError       error

	stoppedHandles []string
	StopError      error

	destroyedHandles []string
	DestroyError     error

	SpawnError error

	LinkError error

	NetInError error

	memoryLimits     []Limit
	limitMemoryError error

	GetMemoryLimitError error

	diskLimits     []DiskLimit
	limitDiskError error

	GetDiskLimitError error

	listCallback ListCallback

	infoError    error
	infoResponse *warden.InfoResponse

	AttachError error

	scriptsThatRan              []*RunningScript
	runCallbacks                map[*RunningScript]RunCallback
	runReturnProcessID          uint32
	runReturnProcessPayloadChan <-chan *warden.ProcessPayload
	runReturnError              error

	copiedIn        []*CopiedIn
	copyInCallbacks map[*CopiedIn]CopyInCallback
	copyInError     error

	copiedOut                     []*CopiedOut
	copyOutCallbacks              map[*CopiedOut]CopyOutCallback
	fileContentToProvideOnCopyOut []byte
	copyOutError                  error

	lock *sync.RWMutex
}

type ListCallback func(filterProperties map[string]string) (*warden.ListResponse, error)

type RunCallback func() (uint32, <-chan *warden.ProcessPayload, error)

type CopyInCallback func(CopiedIn) error
type CopyOutCallback func(CopiedOut) error

type RunningScript struct {
	Handle               string
	Script               string
	ResourceLimits       gordon.ResourceLimits
	EnvironmentVariables []gordon.EnvironmentVariable
}

type CopiedIn struct {
	Handle string
	Src    string
	Dst    string
}

type CopiedOut struct {
	Handle string
	Src    string
	Dst    string
	Owner  string
}

type Limit struct {
	Handle string
	Limit  uint64
}

type DiskLimit struct {
	Handle string
	Limits gordon.DiskLimits
}

func New() *FakeGordon {
	f := &FakeGordon{}
	f.Reset()
	return f
}

func (f *FakeGordon) Reset() {
	f.lock = &sync.RWMutex{}
	f.Connected = false
	f.ConnectError = nil

	f.createdHandles = []string{}
	f.createdProperties = map[string]map[string]string{}
	f.CreateError = nil

	f.stoppedHandles = []string{}
	f.StopError = nil

	f.destroyedHandles = []string{}
	f.DestroyError = nil

	f.SpawnError = nil
	f.LinkError = nil
	f.NetInError = nil
	f.GetMemoryLimitError = nil
	f.GetDiskLimitError = nil
	f.AttachError = nil

	f.infoError = nil

	f.limitMemoryError = nil
	f.limitDiskError = nil
	f.memoryLimits = []Limit{}
	f.diskLimits = []DiskLimit{}

	f.scriptsThatRan = make([]*RunningScript, 0)
	f.runCallbacks = make(map[*RunningScript]RunCallback)
	f.runReturnProcessID = 0
	f.runReturnError = nil

	f.copyInError = nil
	f.copyOutError = nil
	f.copiedIn = []*CopiedIn{}
	f.copiedOut = []*CopiedOut{}
	f.copyInCallbacks = make(map[*CopiedIn]CopyInCallback)
	f.copyOutCallbacks = make(map[*CopiedOut]CopyOutCallback)
	f.fileContentToProvideOnCopyOut = []byte{}
}

func (f *FakeGordon) Connect() error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.Connected = true
	return f.ConnectError
}

func (f *FakeGordon) Create(properties map[string]string) (*warden.CreateResponse, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.CreateError != nil {
		return nil, f.CreateError
	}

	handleUuid, _ := uuid.NewV4()
	handle := handleUuid.String()[:11]

	f.createdHandles = append(f.createdHandles, handle)

	f.createdProperties[handle] = properties

	return &warden.CreateResponse{
		Handle: proto.String(handle),
	}, nil
}

func (f *FakeGordon) CreatedHandles() []string {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.createdHandles
}

func (f *FakeGordon) CreatedProperties(handle string) map[string]string {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.createdProperties[handle]
}

func (f *FakeGordon) Stop(handle string, background, kill bool) (*warden.StopResponse, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.StopError != nil {
		return nil, f.StopError
	}

	f.stoppedHandles = append(f.stoppedHandles, handle)

	return &warden.StopResponse{}, nil
}

func (f *FakeGordon) StoppedHandles() []string {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.stoppedHandles
}

func (f *FakeGordon) Destroy(handle string) (*warden.DestroyResponse, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.DestroyError != nil {
		return nil, f.DestroyError
	}

	f.destroyedHandles = append(f.destroyedHandles, handle)

	return &warden.DestroyResponse{}, nil
}

func (f *FakeGordon) DestroyedHandles() []string {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.destroyedHandles
}

func (f *FakeGordon) NetIn(handle string) (*warden.NetInResponse, error) {
	panic("NOOP!")
	return nil, f.NetInError
}

func (f *FakeGordon) MemoryLimits() []Limit {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.memoryLimits
}

func (f *FakeGordon) SetLimitMemoryError(err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.limitMemoryError = err
}

func (f *FakeGordon) LimitMemory(handle string, limit uint64) (*warden.LimitMemoryResponse, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.memoryLimits = append(f.memoryLimits, Limit{
		Handle: handle,
		Limit:  limit,
	})

	return nil, f.limitMemoryError
}

func (f *FakeGordon) GetMemoryLimit(handle string) (uint64, error) {
	panic("NOOP!")
	return 0, f.GetMemoryLimitError
}

func (f *FakeGordon) DiskLimits() []DiskLimit {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.diskLimits
}

func (f *FakeGordon) SetLimitDiskError(err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.limitDiskError = err
}

func (f *FakeGordon) LimitDisk(handle string, limits gordon.DiskLimits) (*warden.LimitDiskResponse, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.diskLimits = append(f.diskLimits, DiskLimit{
		Handle: handle,
		Limits: limits,
	})

	return nil, f.limitDiskError
}

func (f *FakeGordon) GetDiskLimit(handle string) (uint64, error) {
	panic("NOOP!")
	return 0, f.GetDiskLimitError
}

func (f *FakeGordon) List(filterProperties map[string]string) (*warden.ListResponse, error) {
	f.lock.RLock()
	callback := f.listCallback
	f.lock.RUnlock()

	if callback != nil {
		return callback(filterProperties)
	}

	return &warden.ListResponse{}, nil
}

func (f *FakeGordon) Info(handle string) (*warden.InfoResponse, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.infoResponse, f.infoError
}

func (f *FakeGordon) SetInfoError(err error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.infoError = err
}

func (f *FakeGordon) SetInfoResponse(response *warden.InfoResponse) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.infoResponse = response
}

func (f *FakeGordon) CopyIn(handle, src, dst string) (*warden.CopyInResponse, error) {
	f.lock.RLock()
	err := f.copyInError
	f.lock.RUnlock()

	if err != nil {
		return nil, err
	}

	copiedIn := CopiedIn{
		Handle: handle,
		Src:    src,
		Dst:    dst,
	}

	f.lock.Lock()

	f.copiedIn = append(f.copiedIn, &copiedIn)

	f.lock.Unlock()

	for ci, cb := range f.copyInCallbacks {
		if (ci.Handle == "" || ci.Handle == handle) && ci.Src == src {
			err := cb(copiedIn)
			if err != nil {
				return nil, err
			}
		}
	}

	return &warden.CopyInResponse{}, nil
}

func (f *FakeGordon) ThingsCopiedIn() []*CopiedIn {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.copiedIn
}

func (f *FakeGordon) SetCopyInErr(err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.copyInError = err
}

func (f *FakeGordon) CopyOut(handle, src, dst, owner string) (*warden.CopyOutResponse, error) {
	f.lock.RLock()
	err := f.copyOutError
	f.lock.RUnlock()

	if err != nil {
		return nil, err
	}

	copiedOut := CopiedOut{
		Handle: handle,
		Src:    src,
		Dst:    dst,
		Owner:  owner,
	}

	f.lock.Lock()

	f.copiedOut = append(f.copiedOut, &copiedOut)

	if len(f.fileContentToProvideOnCopyOut) > 0 {
		err := ioutil.WriteFile(dst, f.fileContentToProvideOnCopyOut, os.ModePerm)
		if err != nil {
			panic(err)
		}
	}

	f.lock.Unlock()

	for co, cb := range f.copyOutCallbacks {
		if (co.Handle == "" || co.Handle == handle) && co.Src == src {
			err := cb(copiedOut)
			if err != nil {
				return nil, err
			}
		}
	}

	return &warden.CopyOutResponse{}, nil
}

func (f *FakeGordon) ThingsCopiedOut() []*CopiedOut {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.copiedOut
}

func (f *FakeGordon) SetCopyOutErr(err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.copyOutError = err
}

func (f *FakeGordon) SetCopyOutFileContent(data []byte) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.fileContentToProvideOnCopyOut = data
}

func (f *FakeGordon) Attach(handle string, jobID uint32) (<-chan *warden.ProcessPayload, error) {
	panic("NOOP!")
	return nil, f.AttachError
}

func (f *FakeGordon) ScriptsThatRan() []*RunningScript {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.scriptsThatRan
}

func (f *FakeGordon) SetRunReturnValues(processID uint32, processPayloadChan <-chan *warden.ProcessPayload, err error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.runReturnProcessID = processID
	f.runReturnProcessPayloadChan = processPayloadChan
	f.runReturnError = err
}

func (f *FakeGordon) WhenRunning(handle string, script string, resourceLimits gordon.ResourceLimits, environmentVariables []gordon.EnvironmentVariable, callback RunCallback) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.runCallbacks[&RunningScript{handle, script, resourceLimits, environmentVariables}] = callback
}

func (f *FakeGordon) WhenListing(callback ListCallback) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.listCallback = callback
}

func (f *FakeGordon) WhenCopyingOut(copiedOut CopiedOut, callback CopyOutCallback) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.copyOutCallbacks[&copiedOut] = callback
}

func (f *FakeGordon) WhenCopyingIn(copiedIn CopiedIn, callback CopyInCallback) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.copyInCallbacks[&copiedIn] = callback
}

func (f *FakeGordon) Run(handle string, script string, resourceLimits gordon.ResourceLimits, environmentVariables []gordon.EnvironmentVariable) (uint32, <-chan *warden.ProcessPayload, error) {
	f.lock.Lock()

	f.scriptsThatRan = append(f.scriptsThatRan, &RunningScript{
		Handle:               handle,
		Script:               script,
		ResourceLimits:       resourceLimits,
		EnvironmentVariables: environmentVariables,
	})

	f.lock.Unlock()

	for ro, cb := range f.runCallbacks {
		if (ro.Handle == "" || ro.Handle == handle) && ro.Script == script {
			return cb()
		}
	}

	return f.runReturnProcessID, f.runReturnProcessPayloadChan, f.runReturnError
}
