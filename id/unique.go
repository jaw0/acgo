// Copyright (c) 2018
// Author: Jeff Weisberg <jaw @ tcp4me.com>
// Created: 2018-Jul-07 11:20 (EST)
// Function: ac style unique id

// id - AC style unique ids
package id

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"net"
	"os"
	"sync/atomic"
	"time"
)

var myaddr uint32
var mypid uint16
var seqno uint32

var idEncoder = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

func init() {
	learnAddr()
	mypid = uint16(os.Getpid())
}

// Unique() returns a unique id
func Unique() string {

	buf := new(bytes.Buffer)

	a, b, c, d := scramble(uint32(time.Now().Unix()), myaddr, mypid, seqNext())

	binary.Write(buf, binary.LittleEndian, a)
	binary.Write(buf, binary.LittleEndian, b)
	binary.Write(buf, binary.LittleEndian, c)
	binary.Write(buf, binary.LittleEndian, d)

	// 96 bits base32 encoded leaves 4 bits of zero padding
	// add an extra byte + remove it, to randomize the padding
	// otherwise the id will always end in 'q' or 'a'
	binary.Write(buf, binary.LittleEndian, uint8(d))
	id := idEncoder.EncodeToString(buf.Bytes())

	return id[:20]
}

// UniqueN(n) returns a unique id not shorter than the requested length
func UniqueN(n int) string {

	u := Unique()
	l := len(u)
	if l < n {
		u += RandomText(n - l)
	}

	return u
}

// RandomText(len) - returns random text of the specified length
func RandomText(l int) string {

	r := make([]byte, (l*5+7)/8)
	rand.Read(r)

	s := idEncoder.EncodeToString(r)
	return s[0:l]
}

func RandomBytes(l int) []byte {

	r := make([]byte, l)
	rand.Read(r)
	return r
}

func seqNext() uint16 {

	return uint16(atomic.AddUint32(&seqno, 1))
}

func learnAddr() {

	addrs, _ := net.InterfaceAddrs()
	var ip4, ip16 net.IP

	for _, addr := range addrs {
		ip, _, _ := net.ParseCIDR(addr.String())
		if ip == nil {
			continue
		}

		if ip.IsLoopback() {
			continue
		}

		v4 := ip.To4()

		if len(v4) == 4 {
			if v4.IsGlobalUnicast() || !ip4.IsGlobalUnicast() {
				ip4 = v4
			}
		} else {
			if ip.IsGlobalUnicast() || !ip16.IsGlobalUnicast() {
				ip16 = ip
			}
		}
	}

	if ip4 != nil {
		myaddr = packAddr(ip4)
		return
	}

	if ip16 != nil {
		myaddr = packAddr(ip16[12:16])
		return
	}

	// use a random value
	r := make([]byte, 4)
	rand.Read(r)
	myaddr = packAddr(r)
}

func packAddr(a []byte) uint32 {

	var n uint32
	for _, v := range a {
		n = n<<8 | uint32(v)
	}

	return n
}

// simple 2-round 96-bit feistel
func scramble(a uint32, b uint32, c uint16, d uint16) (uint32, uint32, uint16, uint16) {

	h := uint64(a) | uint64(c)<<32
	l := uint64(b) | uint64(d)<<32

	h, l = scrambleStep(h, l)
	h, l = scrambleStep(h, l)

	x := uint16(h >> 32)
	y := uint16(l >> 32)

	//fmt.Printf("=> %x %x %x %x => %x %x %x %x\n", a, b, c, d, uint32(l), uint32(h), y, x)
	return uint32(l), uint32(h), y, x

}

const fortyeightbits = (1 << 48) - 1

func scrambleStep(h, l uint64) (uint64, uint64) {

	f := l | 0x100000420008

	f *= 0xcc9e2d51
	f += f >> 48
	f += 0xe6546b64
	f = (f << 49) | (f >> 15)
	f *= (f >> 32) ^ f
	f += f >> 48

	f ^= (l >> 24) | (l << 24)
	f &= fortyeightbits

	return l, (f ^ h)
}
