// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license.  See the LICENSE file for details.

package test

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/daijingjing/modbus"
)

func TestMain(t *testing.M) {
	listener, _ := net.Listen("tcp4", ":1000")
	done := make(chan interface{})
	go func() {
		for {

			select {
			case <-done:
				return

			default:
				conn, err := listener.Accept()
				if err != nil {
					log.Println(err)
					continue
				}

				go func() {
					defer conn.Close()
					handler := modbus.NewDTUClientHandler(conn)

					data := make([]byte, 1024)

					for {
						select {
						case <-done:
							return
						default:
							n, _ := io.ReadAtLeast(conn, data, 4)

							handler.SlaveId = data[0]
							pdu, _ := handler.Decode(data[:n])
							length := int(binary.BigEndian.Uint16(pdu.Data[2:]))

							resp := make([]byte, length*2+1)
							resp[0] = byte(length * 2)
							adu, _ := handler.Encode(&modbus.ProtocolDataUnit{
								FunctionCode: pdu.FunctionCode,
								Data:         resp,
							})

							n, _ = conn.Write(adu)
						}
					}

				}()
			}
		}
	}()

	r := t.Run()
	done <- true

	os.Exit(r)
}

func TestClientTestReadHoldingRegisters(t *testing.T) {
	conn, _ := net.Dial("tcp4", "localhost:1000")
	handler := modbus.NewDTUClientHandler(conn)
	handler.Timeout = 5 * time.Second
	handler.SlaveId = 1
	handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)

	client := modbus.NewClient(handler)

	handler.SlaveId = 1
	if data, err := client.ReadHoldingRegisters(0, 2); err != nil {
		t.Fatal(err)
	} else {
		log.Printf("ReadHoldingRegisters: % x", data)
	}

	handler.SlaveId = 2
	if data, err := client.ReadHoldingRegisters(0, 2); err != nil {
		t.Fatal(err)
	} else {
		log.Printf("ReadHoldingRegisters: % x", data)
	}
}

func TestDtuClient(t *testing.T) {
	conn, _ := net.Dial("tcp4", "localhost:1000")
	client := modbus.DTUClient(conn)
	ClientTestAll(t, client)
}

func TestDtuClientAdvancedUsage(t *testing.T) {
	conn, _ := net.Dial("tcp4", "localhost:1000")
	handler := modbus.NewDTUClientHandler(conn)
	handler.Timeout = 5 * time.Second
	handler.SlaveId = 1
	handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)

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
