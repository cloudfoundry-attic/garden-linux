package bridgemgr

import (
	"strconv"
	"time"
)

type BridgeNameGenerator interface {
	Generate() string
}

type bridgeNameGenerator struct {
	prefix      string
	bridgeNames chan string
}

func NewBridgeNameGenerator(prefix string) *bridgeNameGenerator {
	nameChan := make(chan string)
	randPartLen := uint(15 - len(prefix))

	go func(bridgeNames chan<- string) {
		for bridgeNum := time.Now().UnixNano(); ; bridgeNum++ {
			bridgeName := []byte{}

			var i uint
			for i = 0; i < randPartLen; i++ {
				bridgeName = strconv.AppendInt(
					bridgeName,
					(bridgeNum>>(55-(i+1)*5))&31,
					32,
				)
			}

			bridgeNames <- string(bridgeName)
		}
	}(nameChan)

	return &bridgeNameGenerator{
		prefix:      prefix,
		bridgeNames: nameChan,
	}
}

func (generator *bridgeNameGenerator) Generate() string {
	return generator.prefix + <-generator.bridgeNames
}
