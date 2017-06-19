// Copyright 2016 The go-daylight Authors
// This file is part of the go-daylight library.
//
// The go-daylight library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-daylight library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-daylight library. If not, see <http://www.gnu.org/licenses/>.

package parser

import (
	"fmt"
	"strings"

	"github.com/EGaaS/go-egaas-mvp/packages/lib"
	"github.com/EGaaS/go-egaas-mvp/packages/script"
	"github.com/EGaaS/go-egaas-mvp/packages/smart"
	"github.com/EGaaS/go-egaas-mvp/packages/utils"
	"github.com/EGaaS/go-egaas-mvp/packages/utils/tx"

	"gopkg.in/vmihailenco/msgpack.v2"
)

type NewContractParser struct {
	*Parser
	NewContract *tx.NewContract
}

func (p *NewContractParser) Init() error {
	newContract := &tx.NewContract{}
	if err := msgpack.Unmarshal(p.TxBinaryData, newContract); err != nil {
		return p.ErrInfo(err)
	}
	p.NewContract = newContract
	return nil
}

func (p *NewContractParser) Validate() error {
	err := p.generalCheck(`new_contract`, &p.NewContract.Header)
	if err != nil {
		return p.ErrInfo(err)
	}

	// Check the system limits. You can not send more than X time a day this TX
	// ...

	// Check InputData
	name := p.NewContract.Name
	if off := strings.IndexByte(name, '#'); off > 0 {
		p.NewContract.Name = name[:off]
		address := lib.StringToAddress(name[off+1:])
		if address == 0 {
			return p.ErrInfo(fmt.Errorf(`wrong wallet %s`, name[off+1:]))
		}
		p.TxMaps.Int64["wallet_contract"] = address
	}
	verifyData := map[string]string{"global": "int64", "name": "string"}
	err = p.CheckInputData(verifyData)
	if err != nil {
		return p.ErrInfo(err)
	}

	// must be supplemented
	CheckSignResult, err := utils.CheckSign(p.PublicKeys, p.NewContract.ForSign(), p.TxMap["sign"], false)
	if err != nil {
		return p.ErrInfo(err)
	}
	if !CheckSignResult {
		return p.ErrInfo("incorrect sign")
	}
	prefix, err := GetTablePrefix(p.NewContract.Global, p.NewContract.Header.StateID)
	if err != nil {
		return p.ErrInfo(err)
	}
	if len(p.NewContract.Conditions) > 0 {
		if err := smart.CompileEval(string(p.NewContract.Conditions), uint32(p.NewContract.UserID)); err != nil {
			return p.ErrInfo(err)
		}
	}

	if exist, err := p.Single(`select id from "`+prefix+"_smart_contracts"+`" where name=?`, p.NewContract.Name).Int64(); err != nil {
		return p.ErrInfo(err)
	} else if exist > 0 {
		return p.ErrInfo(fmt.Sprintf("The contract %s already exists", p.NewContract.Name))
	}
	return nil
}

func (p *NewContractParser) Action() error {
	prefix, err := GetTablePrefix(p.NewContract.Global, p.NewContract.Header.StateID)
	if err != nil {
		return p.ErrInfo(err)
	}
	var wallet int64
	if wallet = p.NewContract.UserID; wallet == 0 {
		wallet = p.TxWalletID
	}
	root, err := smart.CompileBlock(p.NewContract.Value, prefix, false, 0)
	if err != nil {
		return p.ErrInfo(err)
	}
	if val, ok := p.TxMaps.Int64["wallet_contract"]; ok {
		wallet = val
	}

	tblid, err := p.selectiveLoggingAndUpd([]string{"name", "value", "conditions", "wallet_id"},
		[]interface{}{p.NewContract.Name, p.NewContract.Value, p.NewContract.Conditions,
			wallet}, prefix+"_smart_contracts", nil, nil, true)
	if err != nil {
		return p.ErrInfo(err)
	}
	for i, item := range root.Children {
		if item.Type == script.ObjContract {
			root.Children[i].Info.(*script.ContractInfo).TableID = utils.StrToInt64(tblid)
		}
	}

	smart.FlushBlock(root)
	return nil
}

func (p *NewContractParser) Rollback() error {
	return p.autoRollback()
}

func (p NewContractParser) Header() *tx.Header {
	return &p.NewContract.Header
}
