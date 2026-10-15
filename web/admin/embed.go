// Package admin 提供随二进制发布的管理页面资源。
package admin

import "embed"

// Files 包含管理页面及其静态脚本，不包含后端逻辑。
//
//go:embed index.html chart.umd.min.js
var Files embed.FS
