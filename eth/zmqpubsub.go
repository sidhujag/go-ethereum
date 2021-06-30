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
	"bytes"
	"github.com/ethereum/go-ethereum/log"
	"github.com/go-zeromq/zmq4"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syscoin/btcd/wire"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
)

type ZMQPubSub struct {
	eth            *Ethereum
	pub            zmq4.Socket
	addBlockSub    zmq4.Socket
	deleteBlockSub zmq4.Socket
	createBlockSub zmq4.Socket
	nevmIndexer    NEVMIndex
	inited         bool
}

func (zmq *ZMQPubSub) Close() {
	if !zmq.inited {
		return
	}
	zmq.pub.Close()
	zmq.addBlockSub.Close()
	zmq.deleteBlockSub.Close()
	zmq.createBlockSub.Close()
}

func (zmq *ZMQPubSub) Init(nevmSubEP, nevmPubEP string) error {
	err := zmq.pub.Listen(nevmPubEP)
	if err != nil {
		log.Error("could not listen on NEVM publisher point", "endpoint", nevmPubEP, "err", err)
		return err
	}
	err = zmq.addBlockSub.Dial(nevmSubEP)
	if err != nil {
		log.Error("could not dial NEVM connect", "endpoint", nevmSubEP, "err", err)
		return err
	}
	err = zmq.deleteBlockSub.Dial(nevmSubEP)
	if err != nil {
		log.Error("could not dial NEVM disconnect", "endpoint", nevmSubEP, "err", err)
		return err
	}
	err = zmq.createBlockSub.Dial(nevmSubEP)
	if err != nil {
		log.Error("could not dial NEVM block", "endpoint", nevmSubEP, "err", err)
		return err
	}

	err = zmq.addBlockSub.SetOption(zmq4.OptionSubscribe, "nevmconnect")
	if err != nil {
		log.Error("could not subscribe to nevmconnect topic", "err", err)
		return err
	}
	err = zmq.deleteBlockSub.SetOption(zmq4.OptionSubscribe, "nevmdisconnect")
	if err != nil {
		log.Error("could not subscribe to nevmdisconnect topic", "err", err)
		return err
	}
	err = zmq.createBlockSub.SetOption(zmq4.OptionSubscribe, "nevmblock")
	if err != nil {
		log.Error("could not subscribe to nevmblock topic", "err", err)
		return err
	}
	go func(zmq *ZMQPubSub) {
		for {
			// Read envelope
			msg, err := zmq.addBlockSub.Recv()
			if err != nil {
				if err.Error() == "context canceled" {
					return
				}
				log.Error("addBlockSub: could not receive message", "err", err)
				continue
			}
			if len(msg.Frames) < 2 {
				log.Error("addBlockSub: Invalid number of message frames", "len", len(msg.Frames))
				continue
			}
			// deserialize NEVM data from wire
			var NEVMBlockWire wire.NEVMBlockWire
			r := bytes.NewReader(msg.Frames[1])
			err = NEVMBlockWire.Deserialize(r)
			if err != nil {
				log.Error("addBlockSub: could not deserialize message", "err", err)
				continue
			}
			// decode the raw block inside of NEVM data
			var block types.Block
			rlp.DecodeBytes(NEVMBlockWire.NEVMBlockData, &block)
			// create NEVMBlockConnect object from deserialized block and NEVM wire data
			NEVMBlockConnect := &NEVMBlockConnect{Block: &block,
				Blockhash: common.BytesToHash(NEVMBlockWire.NEVMBlockHash),
				Sysblockhash: NEVMBlockWire.SYSBlockHash,
				Waitforresponse: NEVMBlockWire.WaitForResponse}
			
			// we need to validate that tx root and receipt root is correct based on the block because SYS will store this information in its coinbase tx
			// and re-send the data with waitforresponse = false on resync, thus we should ensure that they are correct before block is approved
			txRootHash := common.BytesToHash(NEVMBlockWire.TxRoot)
			if txRootHash != block.Root() {
				log.Error("addBlockSub: Transaction Root mismatch", "NEVMBlockWire.TxRoot", txRootHash.String(), "block.Root()", block.Root().String())
				continue
			}
			if NEVMBlockConnect.Blockhash != block.Hash() {
				log.Error("addBlockSub: Blockhash mismatch", "NEVMBlockConnect.Blockhash", NEVMBlockConnect.Blockhash.String(), "block.Hash()", block.Hash().String())
				continue
			}
			// deserialize block connect
			result := "connected"
			errMsg := zmq.nevmIndexer.AddBlock(NEVMBlockConnect, zmq.eth)
			if errMsg != nil {
				result = errMsg.Error()
			}
			msgSend := zmq4.NewMsgFrom([]byte("nevmconnect"), []byte(result))
			log.Info("addBlockSub", "frame0", string(msg.Frames[0]), "frame1", string(msg.Frames[1]))
			zmq.pub.SendMulti(msgSend)
		}
	}(zmq)
	go func(zmq *ZMQPubSub) {
		for {
			// Read envelope
			msg, err := zmq.deleteBlockSub.Recv()
			if err != nil {
				if err.Error() == "context canceled" {
					return
				}
				log.Error("deleteBlockSub: could not receive message", "err", err)
				continue
			}
			if len(msg.Frames) < 2 {
				log.Error("deleteBlockSub: Invalid number of message frames", "len", len(msg.Frames))
				continue
			}
			// deserialize block connect
			result := "disconnected"
			errMsg := zmq.nevmIndexer.DeleteBlock(msg.Frames[1], zmq.eth)
			if errMsg != nil {
				result = errMsg.Error()
			}
			msgSend := zmq4.NewMsgFrom([]byte("nevmdisconnect"), []byte(result))
			log.Info("deleteBlockSub", "frame0", string(msg.Frames[0]), "frame1", string(msg.Frames[1]))
			zmq.pub.SendMulti(msgSend)
		}
	}(zmq)
	go func(zmq *ZMQPubSub) {
		for {
			// Read envelope
			msg, err := zmq.createBlockSub.Recv()
			if err != nil {
				if err.Error() == "context canceled" {
					return
				}
				log.Error("createBlockSub: could not receive message", "err", err)
				continue
			}
			if len(msg.Frames) != 1 {
				log.Error("createBlockSub: Invalid number of message frames", "len", len(msg.Frames))
				continue
			}
			var blockRlp []byte
			for {
				block := zmq.nevmIndexer.CreateBlock(zmq.eth)
				if block != nil {
					blockRlp, _ = rlp.EncodeToBytes(block)
					log.Info("block hash", "block", block.Hash().String())
					break
				}
			}
			
			msgSend := zmq4.NewMsgFrom([]byte("nevmblock"), blockRlp)
			zmq.pub.SendMulti(msgSend)

		}
	}(zmq)
	zmq.inited = true
	return nil
}

func NewZMQPubSub(ethIn *Ethereum, nevmIndexerIn NEVMIndex) *ZMQPubSub {
	ctx := context.Background()
	zmq := &ZMQPubSub{
		eth:            ethIn,
		pub:            zmq4.NewPub(ctx),
		addBlockSub:    zmq4.NewSub(ctx, zmq4.WithID(zmq4.SocketIdentity("addBlockSub"))),
		deleteBlockSub: zmq4.NewSub(ctx, zmq4.WithID(zmq4.SocketIdentity("deleteBlockSub"))),
		createBlockSub: zmq4.NewSub(ctx, zmq4.WithID(zmq4.SocketIdentity("createBlockSub"))),
		nevmIndexer:    nevmIndexerIn,
	}
	return zmq
}
