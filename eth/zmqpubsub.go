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
	"github.com/ethereum/go-ethereum/log"
	"github.com/go-zeromq/zmq4"
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

func (zmq *ZMQPubSub) Init(nevmConnectEP, nevmDisconnectEP, nevmBlockEP, nevmPubEP string) error {
	err := zmq.pub.Listen(nevmPubEP)
	if err != nil {
		log.Error("could not listen on NEVM publisher point", "endpoint", nevmPubEP, "err", err)
		return err
	}
	err = zmq.addBlockSub.Dial(nevmConnectEP)
	if err != nil {
		log.Error("could not dial NEVM connect", "endpoint", nevmConnectEP, "err", err)
		return err
	}
	err = zmq.deleteBlockSub.Dial(nevmDisconnectEP)
	if err != nil {
		log.Error("could not dial NEVM disconnect", "endpoint", nevmDisconnectEP, "err", err)
		return err
	}
	err = zmq.createBlockSub.Dial(nevmBlockEP)
	if err != nil {
		log.Error("could not dial NEVM block", "endpoint", nevmBlockEP, "err", err)
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
				log.Error("addBlockSub: could not receive message", "err", err)
				continue
			}
			log.Info("[%s] %s\n", msg.Frames[0], msg.Frames[1])
			// deserialize block connect
			result := "connected"
			errMsg := zmq.nevmIndexer.AddBlock(nil, zmq.eth)
			if errMsg != nil {
				result = errMsg.Error()
			}
			msgSend := zmq4.NewMsgFrom([]byte("nevmconnect"), []byte(result))
			zmq.pub.SendMulti(msgSend)
		}
	}(zmq)
	go func(zmq *ZMQPubSub) {
		for {
			// Read envelope
			msg, err := zmq.deleteBlockSub.Recv()
			if err != nil {
				log.Error("deleteBlockSub: could not receive message", "err", err)
				continue
			}
			log.Info("[%s] %s\n", msg.Frames[0], msg.Frames[1])
			// deserialize block connect
			result := "disconnected"
			errMsg := zmq.nevmIndexer.DeleteBlock(string(msg.Frames[1]), zmq.eth)
			if errMsg != nil {
				result = errMsg.Error()
			}
			msgSend := zmq4.NewMsgFrom([]byte("nevmdisconnect"), []byte(result))
			zmq.pub.SendMulti(msgSend)
		}
	}(zmq)
	go func(zmq *ZMQPubSub) {
		for {
			// Read envelope
			msg, err := zmq.createBlockSub.Recv()
			if err != nil {
				log.Error("createBlockSub: could not receive message", "err", err)
				continue
			}
			log.Info("[%s] %s\n", msg.Frames[0], msg.Frames[1])
			for {
				block := zmq.nevmIndexer.CreateBlock(zmq.eth)
				if block != nil {
					break
				}
			}
			msgSend := zmq4.NewMsgFrom([]byte("nevmblock"), []byte(nil))
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
