// Copyright (c) 2018
// Author: Jeff Weisberg <jaw @ tcp4me.com>
// Created: 2018-Dec-22 14:40 (EST)
// Function: (minimal) AC rpc protocol

package acrpc

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"

	"github.com/jaw0/acgo/diag"
)

type APC struct {
	Addr    string
	MsgId   uint32
	Timeout time.Duration
	// Secret
}

type acProto struct {
	Version    uint32
	Type       uint32
	AuthLen    uint32
	DataLen    uint32
	ContentLen uint32
	MsgIdNo    uint32
	Flags      uint32
}

const (
	PHVERSION      = 0x41433032
	FLAG_ISREPLY   = 0x1
	FLAG_WANTREPLY = 0x2
	FLAG_ISERROR   = 0x4
	FLAG_DATA_ENCR = 0x8  // not supported
	FLAG_CONT_ENCR = 0x10 // ''
)

type marshalable interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}

var dl = diag.Logger("acrpc")

func (c *APC) sendRequest(conn net.Conn, fn uint32, req marshalable, clen int) error {

	// build request
	data, err := req.Marshal()
	if err != nil {
		dl.Problem("cannot marshal AC/RPC: %v", err)
		return err
	}

	prot := &acProto{
		Version:    PHVERSION,
		Flags:      FLAG_WANTREPLY,
		Type:       fn,
		MsgIdNo:    c.MsgId,
		DataLen:    uint32(len(data)),
		ContentLen: uint32(clen),
	}

	// send request
	//   header, data(protobuf), [content]
	err = binary.Write(conn, binary.BigEndian, prot)
	if err != nil {
		return err
	}
	_, err = conn.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func (c *APC) recvReply(conn net.Conn, res marshalable) (*acProto, error) {

	prot := &acProto{}

	//   header, data(protobuf), content
	err := binary.Read(conn, binary.BigEndian, prot)
	if err != nil {
		return nil, err
	}

	dl.Debug("recvd prot %+v", prot)

	// check prot
	if prot.Version != PHVERSION {
		return nil, errors.New("protocol botched: invalid AC/RPC version")
	}
	if prot.Flags&FLAG_ISREPLY == 0 {
		return nil, errors.New("protocol botched: invalid response")
	}
	if prot.Flags&(FLAG_DATA_ENCR|FLAG_CONT_ENCR) != 0 {
		return prot, errors.New("AC/RPC unsupported encryption algorithm")
	}
	if prot.Flags&FLAG_ISERROR != 0 {
		return prot, errors.New("error flag")
	}

	resdata := make([]byte, prot.DataLen)
	_, err = conn.Read(resdata)
	if err != nil {
		return prot, err
	}

	// unmarshal data
	err = res.Unmarshal(resdata)
	if err != nil {
		return prot, err
	}

	dl.Debug("recvd data %+v", res)

	return prot, nil
}

func (c *APC) Call(fn uint32, req marshalable, res marshalable, content []byte) ([]byte, error) {

	// connect
	dl.Debug("connect to %s", c.Addr)
	conn, err := net.DialTimeout("tcp", c.Addr, c.Timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(c.Timeout))

	// send request
	err = c.sendRequest(conn, fn, req, len(content))
	if err != nil {
		return nil, err
	}

	// send content
	_, err = conn.Write(content)
	if err != nil {
		return nil, err
	}

	// read response
	prot, err := c.recvReply(conn, res)
	if err != nil {
		return nil, err
	}

	// return content
	rcontent := make([]byte, prot.ContentLen)
	_, err = conn.Read(rcontent)
	if err != nil {
		return nil, err
	}

	return rcontent, nil
}

func (c *APC) Put(fn uint32, req marshalable, res marshalable, clen int32, r io.Reader) ([]byte, error) {

	// connect
	dl.Debug("connect to %s", c.Addr)
	conn, err := net.DialTimeout("tcp", c.Addr, c.Timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(c.Timeout))

	// send request
	err = c.sendRequest(conn, fn, req, int(clen))
	if err != nil {
		return nil, err
	}

	// send content
	_, err = io.CopyN(conn, r, int64(clen))
	if err != nil {
		return nil, err
	}

	// read response
	prot, err := c.recvReply(conn, res)
	if err != nil {
		return nil, err
	}

	// return content
	rcontent := make([]byte, prot.ContentLen)
	_, err = conn.Read(rcontent)
	if err != nil {
		return nil, err
	}

	return rcontent, nil
}

// caller must close returned conn
func (c *APC) Get(fn uint32, req marshalable, res marshalable, content []byte) (int, io.ReadCloser, error) {

	// connect
	dl.Debug("connect to %s", c.Addr)
	conn, err := net.DialTimeout("tcp", c.Addr, c.Timeout)
	if err != nil {
		return 0, nil, err
	}
	conn.SetDeadline(time.Now().Add(c.Timeout))

	// send request
	err = c.sendRequest(conn, fn, req, len(content))
	if err != nil {
		conn.Close()
		return 0, nil, err
	}

	// send content
	_, err = conn.Write(content)
	if err != nil {
		conn.Close()
		return 0, nil, err
	}

	// read response
	prot, err := c.recvReply(conn, res)
	if err != nil {
		conn.Close()
		return 0, nil, err
	}

	return int(prot.ContentLen), conn, nil
}
