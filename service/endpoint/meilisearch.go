/*
 * Copyright 2020-2021 the original author(https://github.com/wj596)
 *
 * <p>
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 * </p>
 */
package endpoint

import (
	// "log"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/meilisearch/meilisearch-go"
	"github.com/siddontang/go-mysql/canal"
	"github.com/siddontang/go-mysql/mysql"

	"go-mysql-transfer/global"
	"go-mysql-transfer/metrics"
	"go-mysql-transfer/model"
	// "go-mysql-transfer/service/luaengine"
	"go-mysql-transfer/util/logs"
	"go-mysql-transfer/util/stringutil"
)

type MeilisearchEndpoint struct {
	client *meilisearch.Client
	lock   sync.Mutex

	retryLock sync.Mutex
}

func newMeilisearchEndpoint() *MeilisearchEndpoint {
	r := &MeilisearchEndpoint{}
	return r
}

func (s *MeilisearchEndpoint) Connect() error {
	cfg := global.Cfg()
	client := meilisearch.NewClient(meilisearch.ClientConfig{
		Host:   cfg.MeilisearchHost,
		APIKey: cfg.MeilisearchApiKey,
	})

	s.client = client

	// 创建索引和配置
	for _, rule := range global.RuleInsList() {
		if err := s.createOrUpdateIndex(rule); err != nil {
			return err
		}
	}

	return nil
}

func (s *MeilisearchEndpoint) createOrUpdateIndex(rule *global.Rule) error {
	indexUID := rule.MeilisearchIndex
	
	// 检查索引是否存在
	_, err := s.client.GetIndex(indexUID)
	if err != nil {
		// 索引不存在，创建新索引
		var primaryKey string
		if rule.MeilisearchPrimaryKey != "" {
			primaryKey = rule.MeilisearchPrimaryKey
		} else {
			// 使用数据库主键
			if rule.IsCompositeKey {
				primaryKey = "id" // 组合主键使用默认id
			} else {
				pkColumn := rule.TableInfo.Columns[rule.TableInfo.PKColumns[0]]
				primaryKey = rule.WrapName(pkColumn.Name)
			}
		}

		task, err := s.client.CreateIndex(&meilisearch.IndexConfig{
			Uid:        indexUID,
			PrimaryKey: primaryKey,
		})
		if err != nil {
			return err
		}

		// 等待索引创建完成
		_, err = s.client.WaitForTask(task.TaskUID)
		if err != nil {
			return err
		}
	}

	index := s.client.Index(indexUID)

	// 设置可搜索属性
	if rule.MeilisearchSearchableAttrs != "" {
		searchableAttrs := strings.Split(rule.MeilisearchSearchableAttrs, ",")
		for i, attr := range searchableAttrs {
			searchableAttrs[i] = strings.TrimSpace(attr)
		}
		task, err := index.UpdateSearchableAttributes(&searchableAttrs)
		if err != nil {
			return err
		}
		_, err = s.client.WaitForTask(task.TaskUID)
		if err != nil {
			return err
		}
	}

	// 设置可过滤属性
	if rule.MeilisearchFilterableAttrs != "" {
		filterableAttrs := strings.Split(rule.MeilisearchFilterableAttrs, ",")
		for i, attr := range filterableAttrs {
			filterableAttrs[i] = strings.TrimSpace(attr)
		}
		task, err := index.UpdateFilterableAttributes(&filterableAttrs)
		if err != nil {
			return err
		}
		_, err = s.client.WaitForTask(task.TaskUID)
		if err != nil {
			return err
		}
	}

	// 设置可排序属性
	if rule.MeilisearchSortableAttrs != "" {
		sortableAttrs := strings.Split(rule.MeilisearchSortableAttrs, ",")
		for i, attr := range sortableAttrs {
			sortableAttrs[i] = strings.TrimSpace(attr)
		}
		task, err := index.UpdateSortableAttributes(&sortableAttrs)
		if err != nil {
			return err
		}
		_, err = s.client.WaitForTask(task.TaskUID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *MeilisearchEndpoint) Ping() error {
	if s.client == nil {
		return errors.New("MeiliSearch client not initialized")
	}
	
	// 尝试获取健康状态
	_, err := s.client.Health()
	return err
}

func (s *MeilisearchEndpoint) Consume(from mysql.Position, rows []*model.RowRequest) error {
	for _, row := range rows {
		rule, _ := global.RuleIns(row.RuleKey)
		if rule.TableColumnSize != len(row.Row) {
			logs.Warnf("%s schema mismatching", row.RuleKey)
			continue
		}

		metrics.UpdateActionNum(row.Action, row.RuleKey)

		if rule.LuaEnable() {
			// TODO: 支持Lua脚本
			logs.Warnf("MeiliSearch Lua script not supported yet")
			continue
		} else {
			if err := s.processSingleRow(row, rule); err != nil {
				return err
			}
		}
	}

	logs.Infof("处理完成 %d 条数据", len(rows))
	return nil
}

func (s *MeilisearchEndpoint) processSingleRow(row *model.RowRequest, rule *global.Rule) error {
	index := s.client.Index(rule.MeilisearchIndex)
	
	switch row.Action {
	case canal.InsertAction, canal.UpdateAction:
		kvm := rowMap(row, rule, false)
		
		// 设置主键
		id := primaryKey(row, rule)
		if rule.MeilisearchPrimaryKey != "" {
			kvm[rule.MeilisearchPrimaryKey] = id
		} else {
			if rule.IsCompositeKey {
				kvm["id"] = id
			} else {
				pkColumn := rule.TableInfo.Columns[rule.TableInfo.PKColumns[0]]
				kvm[rule.WrapName(pkColumn.Name)] = id
			}
		}
		
		documents := []map[string]interface{}{kvm}
		task, err := index.AddDocuments(documents)
		if err != nil {
			return err
		}
		
		// 等待任务完成
		_, err = s.client.WaitForTask(task.TaskUID)
		if err != nil {
			return err
		}
		
		logs.Infof("action: %s, index: %s, id: %v, data: %v", row.Action, rule.MeilisearchIndex, id, kvm)
		
	case canal.DeleteAction:
		id := primaryKey(row, rule)
		docID := stringutil.ToString(id)
		
		task, err := index.DeleteDocument(docID)
		if err != nil {
			return err
		}
		
		// 等待任务完成
		_, err = s.client.WaitForTask(task.TaskUID)
		if err != nil {
			return err
		}
		
		logs.Infof("action: %s, index: %s, id: %v", row.Action, rule.MeilisearchIndex, id)
	}
	
	return nil
}

func (s *MeilisearchEndpoint) Stock(rows []*model.RowRequest) int64 {
	if len(rows) == 0 {
		return 0
	}

	var count int64
	for _, row := range rows {
		rule, _ := global.RuleIns(row.RuleKey)
		if rule.TableColumnSize != len(row.Row) {
			logs.Warnf("%s schema mismatching", row.RuleKey)
			continue
		}

		if rule.LuaEnable() {
			logs.Warnf("MeiliSearch Lua script not supported yet")
			continue
		}

		kvm := rowMap(row, rule, false)
		
		// 设置主键
		id := primaryKey(row, rule)
		if rule.MeilisearchPrimaryKey != "" {
			kvm[rule.MeilisearchPrimaryKey] = id
		} else {
			if rule.IsCompositeKey {
				kvm["id"] = id
			} else {
				pkColumn := rule.TableInfo.Columns[rule.TableInfo.PKColumns[0]]
				kvm[rule.WrapName(pkColumn.Name)] = id
			}
		}

		index := s.client.Index(rule.MeilisearchIndex)
		documents := []map[string]interface{}{kvm}
		
		task, err := index.AddDocuments(documents)
		if err != nil {
			logs.Error(errors.ErrorStack(err))
			continue
		}
		
		// 等待任务完成
		_, err = s.client.WaitForTask(task.TaskUID)
		if err != nil {
			logs.Error(errors.ErrorStack(err))
			continue
		}
		
		count++
	}

	return count
}

func (s *MeilisearchEndpoint) Close() {
	// MeiliSearch Go客户端不需要显式关闭
}