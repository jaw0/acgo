// Copyright (c) 2018
// Author: Jeff Weisberg <jaw @ tcp4me.com>
// Created: 2018-Jul-09 11:57 (EST)
// Function:

package id

import (
	"fmt"
	"testing"
)

func TestUnique(t *testing.T) {

	fmt.Printf("a %x; p %x\n", myaddr, mypid)
	fmt.Printf("u %v\n", Unique())
	fmt.Printf("u %v\n", Unique())
	fmt.Printf("u %v\n", Unique())
	fmt.Printf("u24 %v\n", UniqueN(24))

	fmt.Printf("r %s\n", RandomText(1))
	fmt.Printf("r %s\n", RandomText(2))
	fmt.Printf("r %s\n", RandomText(3))
	fmt.Printf("r %s\n", RandomText(4))
	fmt.Printf("r %s\n", RandomText(5))
	fmt.Printf("r %s\n", RandomText(6))
	fmt.Printf("r %s\n", RandomText(7))
	fmt.Printf("r %s\n", RandomText(8))
	fmt.Printf("r %s\n", RandomText(9))

	scramble(0, 0, 0, 1)
	scramble(1, 2, 3, 4)

}

func TestScramble(t *testing.T) {

	fmt.Printf("> %012x\n", xtest(1))
	fmt.Printf("> %012x\n", xtest(2))
	fmt.Printf("> %012x\n", xtest(3))
	fmt.Printf("> %012x\n", xtest(4))
	fmt.Printf("> %012x\n", xtest(0x10000000))
	fmt.Printf("> %012x\n", xtest(0x20000000))
	fmt.Printf("> %012x\n", xtest(0x30000000))
	fmt.Printf("> %012x\n", xtest(0x40000000))

}

func xtest(l uint64) uint64 {

	f := l | 0x100000420008

	f *= 0xcc9e2d51
	f += f >> 48
	f += 0xe6546b64
	f = (f << 49) | (f >> 15)
	f *= (f >> 32) ^ f
	f += f >> 48

	f ^= (l >> 24) | (l << 24)
	f &= (1 << 48) - 1

	return f
}
