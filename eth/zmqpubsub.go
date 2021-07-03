// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package core implements the Ethereum consensus protocol.
package eth

import (
	"context"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/log"
	"github.com/go-zeromq/zmq4"
)

type ZMQRep struct {
	eth            *Ethereum
	rep            zmq4.Socket
	nevmIndexer    NEVMIndex
	inited         bool
}

func (zmq *ZMQRep) Close() {
	if !zmq.inited {
		return
	}
	zmq.rep.Close()
}

func (zmq *ZMQRep) Init(nevmEP string) error {
	err := zmq.rep.Listen(nevmEP)
	if err != nil {
		log.Error("could not listen on NEVM REP point", "endpoint", nevmEP, "err", err)
		return err
	}
	go func(zmq *ZMQRep) {
		for {
			// Read envelope
			msg, err := zmq.rep.Recv()
			if err != nil {
				if err.Error() == "context canceled" {
					return
				}
				log.Error("ZMQ: could not receive message", "err", err)
				continue
			}
			if len(msg.Frames) != 2 {
				log.Error("Invalid number of message frames", "len", len(msg.Frames))
				continue
			}
			strTopic := string(msg.Frames[0]) 
			if strTopic == "nevmcomms" {
				if string(msg.Frames[1]) == "\x00" {
					log.Info("ZMQ: exiting...")
					return
				}
				msgSend := zmq4.NewMsgFrom([]byte("nevmcomms"), []byte("ack"))
				zmq.rep.SendMulti(msgSend)
			} else if strTopic == "nevmconnect" {
				result := "connected"
				// deserialize NEVM data from wire
				var nevmBlockConnect NEVMBlockConnect
				err = nevmBlockConnect.Deserialize(msg.Frames[1])
				if err != nil {
					log.Error("addBlockSub", "err", err)
					result = err.Error()
				} else {
					err = zmq.nevmIndexer.AddBlock(&nevmBlockConnect, zmq.eth)
					if err != nil {
						log.Error("addBlockSub", "err", err)
						result = err.Error()
					}
				}
				msgSend := zmq4.NewMsgFrom([]byte("nevmconnect"), []byte(result))
				zmq.rep.SendMulti(msgSend)
			} else if strTopic == "nevmdisconnect" {
				// deserialize block connect
				result := "disconnected"
				errMsg := zmq.nevmIndexer.DeleteBlock(string(msg.Frames[1]), zmq.eth)
				if errMsg != nil {
					result = errMsg.Error()
				}
				msgSend := zmq4.NewMsgFrom([]byte("nevmdisconnect"), []byte(result))
				log.Info("deleteBlockSub", "frame0", string(msg.Frames[0]), "frame1", hex.EncodeToString(msg.Frames[1]), "res", result)
				zmq.rep.SendMulti(msgSend)
			} else if strTopic == "nevmblock" {
				var nevmBlockConnectBytes []byte
				block := zmq.nevmIndexer.CreateBlock(zmq.eth)
				if block != nil {
					var NEVMBlockConnect NEVMBlockConnect
					nevmBlockConnectBytes, err = NEVMBlockConnect.Serialize(block)
					if err != nil {
						log.Error("createBlockSub", "err", err)
						nevmBlockConnectBytes = make([]byte, 0)
					}
					log.Info("NEVMBlockWire.TxRoot ", "txroot", block.TxHash().String(), "len", len(block.TxHash().Bytes()))
				}
				msgSend := zmq4.NewMsgFrom([]byte("nevmblock"), nevmBlockConnectBytes)
				zmq.rep.SendMulti(msgSend)
			}
		}
	}(zmq)
	zmq.inited = true
	return nil
}

func NewZMQRep(ethIn *Ethereum, nevmIndexerIn NEVMIndex) *ZMQRep {
	ctx := context.Background()
	zmq := &ZMQRep{
		eth:            ethIn,
		rep:            zmq4.NewRep(ctx),
		nevmIndexer:    nevmIndexerIn,
	}
	return zmq
}
