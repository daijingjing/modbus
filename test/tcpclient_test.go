// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license.  See the LICENSE file for details.

package test

import (
	"encoding/binary"
	"log"
	"math"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/daijingjing/modbus"
)

const (
	tcpDevice = "localhost:5020"
)

func TestLocal(t *testing.T) {
	handler := modbus.NewTCPClientHandler("localhost:502")
	handler.Timeout = 10 * time.Second

	handler.SlaveId = 1

	var c = 1
	client := modbus.NewClient(handler)
	for {
		c++
		var err error
		rand.NewSource(time.Now().UnixNano())
		var v = rand.Float32() * 100
		var v2 = uint16(rand.Int())
		handler.SlaveId = byte(c%2 + 1)

		bb := make([]byte, 4)
		binary.BigEndian.PutUint32(bb, math.Float32bits(v))

		_, err = client.WriteSingleRegister(10, v2)
		t.Logf("Write Slave %d uint16 value: %d", handler.SlaveId, v2)

		_, err = client.WriteMultipleRegisters(0, 2, bb)
		if err == nil {
			t.Logf("Write Slave %d value: %.03f", handler.SlaveId, v)
		} else {
			t.Fatalf("write value error: %v", err)
		}
		time.Sleep(time.Second * 5)
	}
}

func TestTCPClient(t *testing.T) {
	client := modbus.TCPClient(tcpDevice)
	ClientTestAll(t, client)
}

func TestTCPClientAdvancedUsage(t *testing.T) {
	handler := modbus.NewTCPClientHandler(tcpDevice)
	handler.Timeout = 5 * time.Second
	handler.SlaveId = 1
	handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)
	handler.Connect()
	defer handler.Close()

	client := modbus.NewClient(handler)
	results, err := client.ReadDiscreteInputs(15, 2)
	if err != nil || results == nil {
		t.Fatal(err, results)
	}
	results, err = client.WriteMultipleRegisters(1, 2, []byte{0, 3, 0, 4})
	if err != nil || results == nil {
		t.Fatal(err, results)
	}
	results, err = client.WriteMultipleCoils(5, 10, []byte{4, 3})
	if err != nil || results == nil {
		t.Fatal(err, results)
	}
}
