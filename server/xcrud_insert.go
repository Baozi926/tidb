// Copyright 2017 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	log "github.com/Sirupsen/logrus"
	"github.com/juju/errors"
	"github.com/pingcap/tidb/xprotocol/expr"
	"github.com/pingcap/tidb/xprotocol/util"
	"github.com/pingcap/tipb/go-mysqlx/Crud"
	"github.com/pingcap/tipb/go-mysqlx/Datatypes"
)

type insertBuilder struct {
	baseBuilder
}

func (ib *insertBuilder) build(payload []byte) (*string, error) {
	var msg Mysqlx_Crud.Insert
	var isRelation bool

	if err := msg.Unmarshal(payload); err != nil {
		return nil, util.ErrXBadMessage
	}

	projectionSize := 1
	if msg.GetDataModel() == Mysqlx_Crud.DataModel_TABLE {
		isRelation = true
		projectionSize = len(msg.Projection)
	}

	sqlQuery := "INSERT INTO "
	sqlQuery += *ib.addCollection(msg.Collection)
	generatedField, err := ib.addProjection(msg.Projection, isRelation)
	if err != nil {
		return nil, err
	}
	sqlQuery += *generatedField

	generatedField, err = ib.addValues(msg.Row, projectionSize, isRelation, msg.GetArgs())
	if err != nil {
		return nil, err
	}
	sqlQuery += *generatedField

	return &sqlQuery, nil
}

func (ib *insertBuilder) addProjection(p []*Mysqlx_Crud.Column, tableDataMode bool) (*string, error) {
	target := ""
	if tableDataMode {
		if len(p) != 0 {
			target += " (" + *p[0].Name
			for _, col := range p {
				target += ","
				target += *col.Name
			}
			target += ")"
		}
	} else {
		if len(p) != 0 {
			return nil, util.ErrorMessage(util.CodeErrXBadProjection, "Invalid projection for document operation")
		}
		target += " (doc)"
	}
	return &target, nil
}

func (ib *insertBuilder) addValues(c []*Mysqlx_Crud.Insert_TypedRow, projectionSize int, isRelation bool, msg []*Mysqlx_Datatypes.Scalar) (*string, error) {
	if len(c) == 0 {
		return nil, util.ErrorMessage(util.CodeErrXBadProjection, "Missing row data for Insert")
	}
	target := " VALUES "

	generatedField, err := ib.addRow(c[0], projectionSize, isRelation, msg)
	if err != nil {
		return nil, err
	}

	target += *generatedField
	for _, row := range c[1:] {
		target += ","
		generatedField, err = ib.addRow(row, projectionSize, isRelation, msg)
		if err != nil {
			return nil, err
		}
		target += *generatedField
	}

	return &target, nil
}

func (ib *insertBuilder) addRow(row *Mysqlx_Crud.Insert_TypedRow, projectionSize int, isRelation bool, msg []*Mysqlx_Datatypes.Scalar) (*string, error) {
	if len(row.GetField()) == 0 || len(row.GetField()) != projectionSize {
		log.Infof("[XUWT] row filed(%d), projection size(%d)", len(row.GetField()), projectionSize)
		return nil, util.ErrorMessage(util.CodeErrXBadInsertData, "Wrong number of fields in row being inserted")
	}
	target := "("
	fields := row.GetField()
	cs := make([]*expr.ConcatExpr, len(fields))
	for i, d := range fields {
		cs[i] = expr.NewConcatExpr(d, isRelation, nil, msg)
	}
	gen, err := expr.AddForEach(cs, expr.AddExpr, 0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	target += *gen
	target += ")"
	return &target, nil
}
