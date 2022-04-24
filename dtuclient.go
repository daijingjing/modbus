// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	dtuMinSize = 4
	dtuMaxSize = 256

	dtuExceptionSize = 5
)

// DTUClientHandler implements Packager and Transporter interface.
type DTUClientHandler struct {
	dtuPackager
	dtuTransporter
}

// NewDTUClientHandler allocates and initializes a DTUClientHandler.
func NewDTUClientHandler(conn net.Conn) *DTUClientHandler {
	handler := &DTUClientHandler{}
	handler.conn = conn
	handler.Timeout = tcpTimeout
	return handler
}

// DTUClient creates RTU client with default handler and given connect string.
func DTUClient(conn net.Conn) Client {
	handler := NewDTUClientHandler(conn)
	return NewClient(handler)
}

// dtuPackager implements Packager interface.
type dtuPackager struct {
	SlaveId byte
}

// Encode encodes PDU in a RTU frame:
//  Slave Address   : 1 byte
//  Function        : 1 byte
//  Data            : 0 up to 252 bytes
//  CRC             : 2 byte
func (mb *dtuPackager) Encode(pdu *ProtocolDataUnit) (adu []byte, err error) {
	length := len(pdu.Data) + 4
	if length > dtuMaxSize {
		err = fmt.Errorf("modbus: length of data '%v' must not be bigger than '%v'", length, dtuMaxSize)
		return
	}
	adu = make([]byte, length)

	adu[0] = mb.SlaveId
	adu[1] = pdu.FunctionCode
	copy(adu[2:], pdu.Data)

	// Append crc
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := crc.value()

	adu[length-1] = byte(checksum >> 8)
	adu[length-2] = byte(checksum)
	return
}

// Verify verifies response length and slave id.
func (mb *dtuPackager) Verify(aduRequest []byte, aduResponse []byte) (err error) {
	length := len(aduResponse)
	// Minimum size (including address, function and CRC)
	if length < dtuMinSize {
		err = fmt.Errorf("modbus: response length '%v' does not meet minimum '%v'", length, dtuMinSize)
		return
	}
	// Slave address must match
	if aduResponse[0] != aduRequest[0] {
		err = fmt.Errorf("modbus: response slave id '%v' does not match request '%v'", aduResponse[0], aduRequest[0])
		return
	}
	return
}

// Decode extracts PDU from RTU frame and verify CRC.
func (mb *dtuPackager) Decode(adu []byte) (pdu *ProtocolDataUnit, err error) {
	length := len(adu)
	// Calculate checksum
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := uint16(adu[length-1])<<8 | uint16(adu[length-2])
	if checksum != crc.value() {
		err = fmt.Errorf("modbus: response crc '%v' does not match expected '%v'", checksum, crc.value())
		return
	}
	// Function code & data
	pdu = &ProtocolDataUnit{}
	pdu.FunctionCode = adu[1]
	pdu.Data = adu[2 : length-2]
	return
}

// dtuTransporter implements Transporter interface.
type dtuTransporter struct {
	// Connect & Read timeout
	Timeout time.Duration
	// Transmission logger
	Logger *log.Logger

	BaudRate int

	// TCP connection
	mu           sync.Mutex
	conn         net.Conn
	closeTimer   *time.Timer
	lastActivity time.Time
}

func (mb *dtuTransporter) Send(aduRequest []byte) (aduResponse []byte, err error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Start the timer to close when idle
	mb.lastActivity = time.Now()

	var timeout time.Time
	if mb.Timeout > 0 {
		timeout = mb.lastActivity.Add(mb.Timeout)
	}
	if err = mb.conn.SetDeadline(timeout); err != nil {
		return
	}

	// Send the request
	mb.logf("modbus: sending % x\n", aduRequest)
	if _, err = mb.conn.Write(aduRequest); err != nil {
		return
	}
	function := aduRequest[1]
	functionFail := aduRequest[1] & 0x80
	bytesToRead := calculateResponseLength(aduRequest)
	//time.Sleep(mb.calculateDelay(len(aduRequest) + bytesToRead))

	var n int
	var n1 int
	var data [dtuMaxSize]byte
	//We first read the minimum length and then read either the full package
	//or the error package, depending on the error status (byte 2 of the response)
	n, err = io.ReadAtLeast(mb.conn, data[:], dtuMinSize)
	if err != nil {
		return
	}
	//if the function is correct
	if data[1] == function {
		//we read the rest of the bytes
		if n < bytesToRead {
			if bytesToRead > dtuMinSize && bytesToRead <= dtuMaxSize {
				if bytesToRead > n {
					n1, err = io.ReadFull(mb.conn, data[n:bytesToRead])
					n += n1
				}
			}
		}
	} else if data[1] == functionFail {
		//for error we need to read 5 bytes
		if n < dtuExceptionSize {
			n1, err = io.ReadFull(mb.conn, data[n:dtuExceptionSize])
		}
		n += n1
	}

	if err != nil {
		_ = mb.flush()
		return
	}
	aduResponse = data[:n]
	mb.logf("modbus: received % x\n", aduResponse)
	return
}

// calculateDelay roughly calculates time needed for the next frame.
// See MODBUS over Serial Line - Specification and Implementation Guide (page 13).
func (mb *dtuTransporter) calculateDelay(chars int) time.Duration {
	var characterDelay, frameDelay int // us

	if mb.BaudRate <= 0 || mb.BaudRate > 19200 {
		characterDelay = 750
		frameDelay = 1750
	} else {
		characterDelay = 15000000 / mb.BaudRate
		frameDelay = 35000000 / mb.BaudRate
	}
	return time.Duration(characterDelay*chars+frameDelay) * time.Microsecond
}

// flush flushes pending data in the connection,
// returns io.EOF if connection is closed.
func (mb *dtuTransporter) flush() (err error) {
	if err = mb.conn.SetReadDeadline(time.Now()); err != nil {
		return
	}
	// Timeout setting will be reset when reading
	b := make([]byte, 1000)
	if _, err = mb.conn.Read(b); err != nil {
		// Ignore timeout error
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			err = nil
		}
	}
	return
}

func (mb *dtuTransporter) logf(format string, v ...interface{}) {
	if mb.Logger != nil {
		mb.Logger.Printf(format, v...)
	}
}
