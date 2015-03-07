package main

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
	return *c
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
	if c.IPNet == nil {
		return ""
	}
	return c.IPNet.String()
}

type IPVar struct {
	net.IP
}

func (i *IPVar) Get() interface{} {
	return IPVar(*i)
}

func (i *IPVar) Set(s string) error {
	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", s)
	}

	i.IP = ip
	return nil
}

func (i *IPVar) String() string {
	return i.IP.String()
}
