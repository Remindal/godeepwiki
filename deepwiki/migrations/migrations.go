// Package migrations 内嵌 PostgreSQL 迁移脚本（变更总纲 §4.1：golang-migrate iofs source，只前进不回滚）。
// 命名从 000001_init.up.sql 起，只有 .up 没有 .down；迁移文件一旦合入不得修改，
// 变更只能新增更高序号的 .up.sql 文件。
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
