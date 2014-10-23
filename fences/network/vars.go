package network

import (
	"fmt"
	"math"
	"net"
	"strconv"
)

type MtuVar uint

func (m *MtuVar) Get() interface{} {
	return MtuVar(*m)
}

func (m *MtuVar) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return err
	}

	if v > math.MaxUint32 {
		return fmt.Errorf("must be less than %d", math.MaxUint32)
	}

	*m = MtuVar(v)
	return nil
}

func (m *MtuVar) String() string {
	return fmt.Sprintf("%v", *m)
}

type CidrVar struct {
	*net.IPNet
}

func cidrVar(s string) CidrVar {
	v := &CidrVar{}
	v.Set(s)
	return *v
}

func (c *CidrVar) Get() interface{} {
	return CidrVar(*c)
}

func (c *CidrVar) Set(s string) error {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		return err
	}

	c.IPNet = network
	return nil
}

func (c *CidrVar) String() string {
	return c.IPNet.String()
}
