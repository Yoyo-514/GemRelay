// Package testutil 提供测试进程的数据库隔离与公共夹具。
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gemrelay/internal/config"
	"gemrelay/internal/storage"
)

// Run 为每个测试包独立设置临时 SQLite，禁止继承生产连接串。
func Run(m *testing.M) int {
	if err := os.Unsetenv("SQL_DSN"); err != nil {
		panic(err)
	}
	dir, err := os.MkdirTemp("", "gemrelay-test-")
	if err != nil {
		panic(err)
	}
	config.Startup().DBPath = filepath.Join(dir, "test.db")
	code := m.Run()
	if err := storage.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	return code
}
