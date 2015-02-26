package subnets

import "github.com/cloudfoundry-incubator/garden-linux/gerr"

var (
	// ErrInsufficientSubnets is returned by AllocateDynamically if no more subnets can be allocated.
	ErrInsufficientSubnets = gerr.New("insufficient subnets remaining in the pool")

	// ErrInsufficientIPs is returned by AllocateDynamically if no more IPs can be allocated.
	ErrInsufficientIPs = gerr.New("insufficient IPs remaining in the pool")

	// ErrReleasedUnallocatedNetwork is returned by Release if the subnet is not allocated.
	ErrReleasedUnallocatedSubnet = gerr.New("subnet is not allocated")

	// ErrOverlapsExistingSubnet is returned if a recovered subnet overlaps an existing, allocated subnet
	ErrOverlapsExistingSubnet = gerr.New("subnet overlaps an existing subnet")

	// ErrInvalidRange is returned by AllocateStatically and by Recover if the subnet range is invalid.
	ErrInvalidRange = gerr.New("subnet has invalid range")

	// ErrInvalidIP is returned if a static IP is requested inside a subnet
	// which does not contain that IP
	ErrInvalidIP = gerr.New("the requested IP is not within the subnet")

	// ErrIPAlreadyAllocated is returned if a static IP is requested which has already been allocated
	ErrIPAlreadyAllocated = gerr.New("the requested IP is already allocated")

	// ErrIpCannotBeNil is returned by Release(..) and Recover(..) if a nil
	// IP address is passed.
	ErrIpCannotBeNil = gerr.New("the IP field cannot be empty")

	ErrIPEqualsGateway   = gerr.New("a container IP must not equal the gateway IP")
	ErrIPEqualsBroadcast = gerr.New("a container IP must not equal the broadcast IP")
)
