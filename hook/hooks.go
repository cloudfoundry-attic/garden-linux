package hook

import "fmt"

type HookSet map[Phase]Hook
type Hook func()

type Phase string

const (
	PARENT_BEFORE_CLONE Phase = "parent-before-clone"
	PARENT_AFTER_CLONE  Phase = "parent-after-clone"
	CHILD_AFTER_PIVOT   Phase = "child-after-pivot"
)

var DefaultHookSet HookSet = make(map[Phase]Hook)

func Main(args []string) {
	DefaultHookSet.Main(Phase(args[0]))
}

func Register(name Phase, fn Hook) {
	DefaultHookSet.Register(name, fn)
}

func (h HookSet) Main(phase Phase) {
	if fn, ok := h[phase]; ok {
		fn()
	} else {
		panic(fmt.Sprintf("hooks: no such hook: %s", phase))
	}
}

func (h HookSet) Register(name Phase, fn Hook) {
	if _, exists := h[name]; exists {
		panic(fmt.Sprintf("hooks: already registered hook: %s", name))
	}

	h[name] = fn
}
