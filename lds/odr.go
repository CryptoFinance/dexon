// Copyright 2016 The go-ethereum Authors
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

package lds

import (
	"context"

	"github.com/dexon-foundation/dexon/core"
	"github.com/dexon-foundation/dexon/ethdb"
	"github.com/dexon-foundation/dexon/light"
	"github.com/dexon-foundation/dexon/log"
)

// LdsOdr implements light.OdrBackend
type LdsOdr struct {
	db                                         ethdb.Database
	indexerConfig                              *light.IndexerConfig
	chtIndexer, bloomTrieIndexer, bloomIndexer *core.ChainIndexer
	retriever                                  *retrieveManager
	stop                                       chan struct{}
}

func NewLdsOdr(db ethdb.Database, config *light.IndexerConfig, retriever *retrieveManager) *LdsOdr {
	return &LdsOdr{
		db:            db,
		indexerConfig: config,
		retriever:     retriever,
		stop:          make(chan struct{}),
	}
}

// Stop cancels all pending retrievals
func (odr *LdsOdr) Stop() {
	close(odr.stop)
}

// Database returns the backing database
func (odr *LdsOdr) Database() ethdb.Database {
	return odr.db
}

// SetIndexers adds the necessary chain indexers to the ODR backend
func (odr *LdsOdr) SetIndexers(chtIndexer, bloomTrieIndexer, bloomIndexer *core.ChainIndexer) {
	odr.chtIndexer = chtIndexer
	odr.bloomTrieIndexer = bloomTrieIndexer
	odr.bloomIndexer = bloomIndexer
}

// ChtIndexer returns the CHT chain indexer
func (odr *LdsOdr) ChtIndexer() *core.ChainIndexer {
	return odr.chtIndexer
}

// BloomTrieIndexer returns the bloom trie chain indexer
func (odr *LdsOdr) BloomTrieIndexer() *core.ChainIndexer {
	return odr.bloomTrieIndexer
}

// BloomIndexer returns the bloombits chain indexer
func (odr *LdsOdr) BloomIndexer() *core.ChainIndexer {
	return odr.bloomIndexer
}

// IndexerConfig returns the indexer config.
func (odr *LdsOdr) IndexerConfig() *light.IndexerConfig {
	return odr.indexerConfig
}

const (
	MsgBlockBodies = iota
	MsgCode
	MsgReceipts
	MsgProofsV1
	MsgProofsV2
	MsgHeaderProofs
	MsgHelperTrieProofs
)

// Msg encodes a LES message that delivers reply data for a request
type Msg struct {
	MsgType int
	ReqID   uint64
	Obj     interface{}
}

// Retrieve tries to fetch an object from the LES network.
// If the network retrieval was successful, it stores the object in local db.
func (odr *LdsOdr) Retrieve(ctx context.Context, req light.OdrRequest) (err error) {
	lreq := LdsRequest(req)

	reqID := genReqID()
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			return lreq.GetCost(dp.(*peer))
		},
		canSend: func(dp distPeer) bool {
			p := dp.(*peer)
			return lreq.CanSend(p)
		},
		request: func(dp distPeer) func() {
			p := dp.(*peer)
			cost := lreq.GetCost(p)
			p.fcServer.QueueRequest(reqID, cost)
			return func() { lreq.Request(reqID, p) }
		},
	}

	if err = odr.retriever.retrieve(ctx, reqID, rq, func(p distPeer, msg *Msg) error { return lreq.Validate(odr.db, msg) }, odr.stop); err == nil {
		// retrieved from network, store in db
		req.StoreResult(odr.db)
	} else {
		log.Debug("Failed to retrieve data from network", "err", err)
	}
	return
}
